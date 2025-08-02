package user

import (
	"context"
	"fmt"
	"github.com/chenmingyong0423/go-mongox/v2"
	"github.com/chenmingyong0423/go-mongox/v2/builder/query"
	"github.com/robbiebyrd/indri/internal/clients/mongodb"
	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/repo/utils"
	"go.mongodb.org/mongo-driver/v2/bson"
)

var collectionName = "user"

type Repo struct {
	ctx        *context.Context
	collection *mongox.Collection[models.User]
	client     *mongodb.Client
}

// NewRepo creates a new repository for accessing user data.
func NewRepo(ctx context.Context, client *mongodb.Client) *Repo {
	userColl := mongox.NewCollection[models.User](client.Database, collectionName)

	return &Repo{
		ctx:        &ctx,
		client:     client,
		collection: userColl,
	}
}

// New creates a new user, given an ID.
func (s *Repo) New(user models.CreateUser) (*models.User, error) {
	matchingUser, _ := s.collection.Finder().Filter(query.Eq("email", user.Email)).FindOne(*s.ctx)

	if matchingUser != nil {
		return nil, fmt.Errorf("a user with email address %v already exists", user.Email)
	}

	doc, err := utils.CreateBSONDoc(user)
	if err != nil {
		return nil, err
	}

	// Create the update document, specifying the fields to update. `nil` fields are not updated,
	// as they are dropped in the conversion. We specify a filter for the requested user ID, so only
	// one document should ever be updated.
	result, err := s.collection.Collection().InsertOne(*s.ctx, &doc)
	if err != nil {
		return nil, err
	}

	// Get the newly inserted ID
	insertedId := result.InsertedID.(bson.ObjectID).Hex()

	// Get the User and return it
	return s.Get(insertedId)
}

// Find retrieves user data records for a specific key/value.
func (s *Repo) Find(key string, value string) ([]*models.User, error) {
	return s.collection.Finder().Filter(query.Eq(key, value)).Find(*s.ctx)
}

// FindFirst retrieves the first user data record, given a key/value.
func (s *Repo) FindFirst(key string, value string) (*models.User, error) {
	return s.collection.Finder().Filter(query.Eq(key, value)).FindOne(*s.ctx)
}

// Get retrieves user data for a specific user ID.
func (s *Repo) Get(id string) (*models.User, error) {
	objectId, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}

	return s.collection.Finder().Filter(query.Id(objectId)).FindOne(*s.ctx)
}

// Exists checks to see if a user with the given ID already exists.
func (s *Repo) Exists(id string) (bool, error) {
	objectId, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return false, err
	}

	count, err := s.collection.Finder().Filter(query.Id(objectId)).Count(*s.ctx)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// Update saves user data to the repository.
func (s *Repo) Update(user *models.UpdateUser) error {
	// Convert the hex-based, string id we get to an actual ObjectID
	objectId, err := bson.ObjectIDFromHex(user.ID)
	if err != nil {
		return err
	}

	doc, err := utils.CreateBSONDoc(user)
	if err != nil {
		return err
	}

	// Create the update document, specifying the fields to update. `nil` fields are not updated,
	// as they are dropped in the conversion. We specify a filter for the requested user ID, so only
	// one document should ever be updated.
	result, err := s.collection.Collection().UpdateOne(
		context.TODO(),
		bson.D{{Key: "_id", Value: objectId}},
		bson.D{{Key: "$set", Value: doc}},
	)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("user with id %v does not exists", objectId)
	}

	return nil
}
