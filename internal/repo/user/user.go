package user

import (
	"context"
	"errors"
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

	return &Store{
		ctx:        &ctx,
		client:     client,
		collection: userColl,
	}, nil
}

// New stores a new user. A taken email is repo.ErrDuplicate, decided by the
// unique index on email rather than by a check two registrations could both
// pass.
func (s *Store) New(c models.CreateUser) (*models.User, error) {
	u := newUser(c)

	doc, err := repoUtils.CreateBSONDoc(u)
	if err != nil {
		return nil, err
	}

	if _, err := s.collection.Collection().InsertOne(*s.ctx, &doc); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil, fmt.Errorf("a user with email %q: %w", c.Email, repoErrors.ErrDuplicate)
		}

		return nil, err
	}

	return s.Get(u.ID)
}

// Find retrieves every user whose key field equals value.
func (s *Store) Find(key string, value string) ([]*models.User, error) {
	if err := requireLookup(key); err != nil {
		return nil, err
	}

	return s.collection.Finder().Filter(query.Eq(key, value)).Find(*s.ctx)
}

// FindFirst retrieves the first user whose key field equals value.
func (s *Store) FindFirst(key string, value string) (*models.User, error) {
	if err := requireLookup(key); err != nil {
		return nil, err
	}

	u, err := s.collection.Finder().Filter(query.Eq(key, value)).FindOne(*s.ctx)

	return u, notFound(err, fmt.Sprintf("no user with %s %q", key, value))
}

// Get retrieves the user with this id.
func (s *Store) Get(id string) (*models.User, error) {
	u, err := s.collection.Finder().Filter(query.Id(id)).FindOne(*s.ctx)

	return u, notFound(err, fmt.Sprintf("user %q", id))
}

// Exists checks to see if a user with the given ID already exists.
func (s *Store) Exists(id string) (bool, error) {
	count, err := s.collection.Finder().Filter(query.Id(id)).Count(*s.ctx)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// Update sets the fields the update carries; the BSON form omits the rest, so
// an edit that does not mention the password keeps it.
func (s *Store) Update(update *models.UpdateUser) error {
	if err := requireID(update); err != nil {
		return err
	}

	update.UpdatedAt = time.Now()

	doc, err := repoUtils.CreateBSONDoc(update)
	if err != nil {
		return err
	}

	result, err := s.collection.Collection().UpdateOne(
		*s.ctx,
		bson.D{{Key: "_id", Value: update.ID}},
		bson.D{{Key: "$set", Value: doc}},
	)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return fmt.Errorf("a user with email %q: %w", update.Email, repoErrors.ErrDuplicate)
		}

		return err
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("user %q: %w", update.ID, repoErrors.ErrNotFound)
	}

	return nil
}

// notFound maps the driver's "no document" onto repo.ErrNotFound, so callers
// test one sentinel whichever backend they run on.
func notFound(err error, what string) error {
	if errors.Is(err, mongo.ErrNoDocuments) {
		return fmt.Errorf("%s: %w", what, repoErrors.ErrNotFound)
	}

	return err
}
