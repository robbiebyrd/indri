package user

import (
	"context"
	"fmt"
	"time"

	"github.com/chenmingyong0423/go-mongox/v2"
	"github.com/chenmingyong0423/go-mongox/v2/builder/query"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/robbiebyrd/indri/internal/clients/mongodb"
	"github.com/robbiebyrd/indri/internal/models"
	repoUtils "github.com/robbiebyrd/indri/internal/repo/utils"
)

var collectionName = "user"

type Store struct {
	ctx        *context.Context
	collection *mongox.Collection[models.User]
	client     *mongodb.Client
}

// NewStore creates a new repository for accessing user data.
func NewStore(ctx context.Context, client *mongodb.Client) (*Store, error) {
	userColl := mongox.NewCollection[models.User](client.Database, collectionName)

	indexModels := []mongo.IndexModel{
		{
			Keys: bson.D{
				{"email", 1},
			},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{
				{"name", 1},
			},
		},
		{
			Keys: bson.D{
				{"score", 1},
			},
		},
	}

	_, err := userColl.Collection().Indexes().CreateMany(ctx, indexModels)
	if err != nil {
		return nil, err
	}

	return &Store{
		ctx:        &ctx,
		client:     client,
		collection: userColl,
	}, nil
}

// New creates a new user, given an ID.
func (s *Store) New(user models.CreateUser) (*models.User, error) {
	matchingUser, _ := s.collection.Finder().Filter(query.Eq("email", user.Email)).FindOne(*s.ctx)

	if matchingUser != nil {
		return nil, fmt.Errorf("a user with email address %v already exists", user.Email)
	}

	user.CreatedAt = time.Now()

	doc, err := repoUtils.CreateBSONDoc(user)
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
func (s *Store) Find(key string, value string) ([]*models.User, error) {
	return s.collection.Finder().Filter(query.Eq(key, value)).Find(*s.ctx)
}

// FindFirst retrieves the first user data record, given a key/value.
func (s *Store) FindFirst(key string, value string) (*models.User, error) {
	return s.collection.Finder().Filter(query.Eq(key, value)).FindOne(*s.ctx)
}

// Get retrieves user data for a specific user ID.
func (s *Store) Get(id string) (*models.User, error) {
	objectId, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}

	return s.collection.Finder().Filter(query.Id(objectId)).FindOne(*s.ctx)
}

// Exists checks to see if a user with the given ID already exists.
func (s *Store) Exists(id string) (bool, error) {
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
func (s *Store) Update(user *models.UpdateUser) error {
	// Convert the hex-based, string id we get to an actual ObjectID
	objectId, err := bson.ObjectIDFromHex(user.ID)
	if err != nil {
		return err
	}

	user.UpdatedAt = time.Now()

	doc, err := repoUtils.CreateBSONDoc(user)
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
