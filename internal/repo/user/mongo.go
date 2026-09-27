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
	repoErrors "github.com/robbiebyrd/indri/internal/repo"
	"github.com/robbiebyrd/indri/internal/repo/ids"
	repoUtils "github.com/robbiebyrd/indri/internal/repo/utils"
)

var collectionName = "user"

type MongoStore struct {
	ctx        *context.Context
	collection *mongox.Collection[models.User]
	client     *mongodb.Client
}

// NewMongoStore creates a new repository for accessing user data.
func NewMongoStore(ctx context.Context, client *mongodb.Client) (*MongoStore, error) {
	userColl := mongox.NewCollection[models.User](client.Database, collectionName)

	indexModels := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "email", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{Key: "name", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "score", Value: 1}},
		},
	}

	_, err := userColl.Collection().Indexes().CreateMany(ctx, indexModels)
	if err != nil {
		return nil, err
	}

	return &MongoStore{
		ctx:        &ctx,
		client:     client,
		collection: userColl,
	}, nil
}

// New creates a new user with a pre-generated UUID.
func (s *MongoStore) New(user models.CreateUser) (*models.User, error) {
	matchingUser, _ := s.collection.Finder().Filter(query.Eq("email", user.Email)).FindOne(*s.ctx)

	if matchingUser != nil {
		return nil, fmt.Errorf("a user with email address %v already exists: %w", user.Email, repoErrors.ErrDuplicate)
	}

	now := time.Now()
	newUser := &models.User{
		ID:          ids.New(),
		CreatedAt:   now,
		UpdatedAt:   now,
		Email:       user.Email,
		Name:        user.Name,
		DisplayName: user.DisplayName,
		Password:    user.Password,
	}

	doc, err := repoUtils.CreateBSONDoc(newUser)
	if err != nil {
		return nil, err
	}

	if _, err := s.collection.Collection().InsertOne(*s.ctx, &doc); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil, fmt.Errorf("a user with email address %v already exists: %w", user.Email, repoErrors.ErrDuplicate)
		}
		return nil, err
	}

	return s.Get(newUser.ID)
}

// Find retrieves user data records for a specific key/value.
func (s *MongoStore) Find(key string, value string) ([]*models.User, error) {
	return s.collection.Finder().Filter(query.Eq(key, value)).Find(*s.ctx)
}

// FindFirst retrieves the first user data record, given a key/value.
func (s *MongoStore) FindFirst(key string, value string) (*models.User, error) {
	return s.collection.Finder().Filter(query.Eq(key, value)).FindOne(*s.ctx)
}

// Get retrieves user data for a specific user ID.
func (s *MongoStore) Get(id string) (*models.User, error) {
	return s.collection.Finder().Filter(bson.D{{Key: "_id", Value: id}}).FindOne(*s.ctx)
}

// Exists checks to see if a user with the given ID already exists.
func (s *MongoStore) Exists(id string) (bool, error) {
	count, err := s.collection.Finder().Filter(bson.D{{Key: "_id", Value: id}}).Count(*s.ctx)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// Update saves user data to the repository.
func (s *MongoStore) Update(user *models.UpdateUser) error {
	user.UpdatedAt = time.Now()

	doc, err := repoUtils.CreateBSONDoc(user)
	if err != nil {
		return err
	}

	result, err := s.collection.Collection().UpdateOne(
		context.TODO(),
		bson.D{{Key: "_id", Value: user.ID}},
		bson.D{{Key: "$set", Value: doc}},
	)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("user with id %v does not exists", user.ID)
	}

	return nil
}
