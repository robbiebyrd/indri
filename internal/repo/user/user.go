package user

import (
	"context"
	"fmt"
	"github.com/chenmingyong0423/go-mongox/v2"
	"github.com/chenmingyong0423/go-mongox/v2/builder/query"
	"go.mongodb.org/mongo-driver/v2/bson"
	"indri/internal/clients/mongodb"
	"indri/internal/models"
)

type Repo struct {
	ctx        *context.Context
	collection *mongox.Collection[models.User]
	client     *mongodb.Client
}

// NewRepo creates a new repository for accessing user data.
func NewRepo() *Repo {
	client, err := mongodb.New()
	if err != nil {
		panic(err)
	}

	userColl := mongox.NewCollection[models.User](client.Database, "users")
	ctx := context.Background()

	return &Repo{
		ctx:        &ctx,
		client:     client,
		collection: userColl,
	}
}

// New creates a new user, given an ID.
func (s *Repo) New(user models.User) (*models.User, error) {
	matchingUser, _ := s.collection.Finder().Filter(query.Eq("email", user.Email)).FindOne(*s.ctx)

	if matchingUser != nil {
		return nil, fmt.Errorf("a user with email address %v already exists", user.Email)
	}

	g, err := s.collection.Creator().InsertOne(*s.ctx, &user)
	if err != nil {
		return nil, err
	}

	createdUser, err := s.collection.Finder().Filter(query.Id(g.InsertedID)).FindOne(*s.ctx)
	if err != nil {
		return nil, err
	}

	return createdUser, nil
}

// Find retrieves user data for a specific user ID.
func (s *Repo) Find(key string, value string) ([]*models.User, error) {
	return s.collection.Finder().Filter(query.Eq(key, value)).Find(*s.ctx)
}

// Find retrieves user data for a specific user ID.
func (s *Repo) FindFirst(key string, value string) (*models.User, error) {
	return s.collection.Finder().Filter(query.Eq(key, value)).FindOne(*s.ctx)
}

// Get retrieves user data for a specific user ID.
func (s *Repo) Get(id string) (*models.User, error) {
	return s.collection.Finder().Filter(query.Id(id)).FindOne(*s.ctx)
}

// Exists checks to see if a user with the given ID already exists.
func (s *Repo) Exists(id string) (bool, error) {
	count, err := s.collection.Finder().Filter(query.Id(id)).Count(*s.ctx)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// Update saves user data to the repository.
func (s *Repo) Update(user *models.User) error {
	g, err := s.collection.Collection().ReplaceOne(context.Background(), bson.M{"id": user.ID}, user)
	if err != nil {
		panic(err)
	}

	if g == nil {
		return fmt.Errorf("couldn't find user with email %v", user.ID)
	}

	return nil
}
