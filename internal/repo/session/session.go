package session

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/chenmingyong0423/go-mongox/v2"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/robbiebyrd/indri/internal/clients/mongodb"
	"github.com/robbiebyrd/indri/internal/models"
	repoErrors "github.com/robbiebyrd/indri/internal/repo"
	repoUtils "github.com/robbiebyrd/indri/internal/repo/utils"
)

var collectionName = "session"

// sessionMaxAge is an absolute cap on how long a session (and its bearer token)
// remains valid, enforced by a TTL index as defense in depth: even a session
// that is never explicitly logged out expires and its token stops working.
const sessionMaxAge = 7 * 24 * time.Hour

type Store struct {
	ctx        *context.Context
	collection *mongox.Collection[models.Session]
	client     *mongodb.Client
}

// NewStore creates a new repository for accessing user data.
func NewStore(ctx context.Context, client *mongodb.Client) (*Store, error) {
	sessionColl := mongox.NewCollection[models.Session](client.Database, collectionName)

	indexModels := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "userId", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys:    bson.D{{Key: "token", Value: 1}},
			Options: options.Index().SetUnique(true).SetSparse(true),
		},
		{
			// TTL index: MongoDB removes a session sessionMaxAge after its
			// createdAt, capping how long any token can be replayed.
			Keys:    bson.D{{Key: "createdAt", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(int32(sessionMaxAge.Seconds())),
		},
		{
			Keys: bson.D{
				{Key: "userId", Value: 1},
				{Key: "gameId", Value: 1},
			},
		},
		{
			Keys: bson.D{
				{Key: "gameId", Value: 1},
				{Key: "userId", Value: 1},
				{Key: "teamId", Value: 1},
			},
		},
	}

	_, err := sessionColl.Collection().Indexes().CreateMany(ctx, indexModels)
	if err != nil {
		return nil, err
	}

	return &Store{
		ctx:        &ctx,
		client:     client,
		collection: sessionColl,
	}, nil
}

// liveFilter matches the session filter describes, provided it has not expired.
// The TTL index deletes expired sessions eventually — MongoDB sweeps once a
// minute — so every read checks the cutoff itself: a token is dead at
// sessionMaxAge, not whenever the sweep gets to it.
func liveFilter(filter bson.D) bson.D {
	return append(filter, bson.E{Key: "createdAt", Value: bson.D{{Key: "$gt", Value: expiryCutoff()}}})
}

func (s *Store) findOne(filter bson.D, what string) (*models.Session, error) {
	sess, err := s.collection.Finder().Filter(liveFilter(filter)).FindOne(*s.ctx)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, fmt.Errorf("%s: %w", what, repoErrors.ErrNotFound)
	}

	return sess, err
}

// New returns the user's session if they have one, or creates it. An expired
// session of theirs is deleted first, so it can neither be handed back nor
// block the new one through the unique index on userId.
func (s *Store) New(c models.CreateSession) (*models.Session, error) {
	if c.UserID == "" {
		return nil, errNoUser
	}

	if _, err := s.collection.Collection().DeleteMany(*s.ctx, bson.D{
		{Key: "userId", Value: c.UserID},
		{Key: "createdAt", Value: bson.D{{Key: "$lte", Value: expiryCutoff()}}},
	}); err != nil {
		return nil, fmt.Errorf("purging the expired session of user %q: %w", c.UserID, err)
	}

	existing, err := s.FindFirst("userId", c.UserID)
	if err == nil {
		return existing, nil
	}

	if !errors.Is(err, repoErrors.ErrNotFound) {
		return nil, err
	}

	created := newSession(c)

	doc, err := repoUtils.CreateBSONDoc(created)
	if err != nil {
		return nil, err
	}

	if _, err := s.collection.Collection().InsertOne(*s.ctx, &doc); err != nil {
		// Lost a race with a concurrent login for the same user: the unique
		// userId index rejected this insert. One session per user, so the
		// winner's is the answer.
		if mongo.IsDuplicateKeyError(err) {
			return s.FindFirst("userId", c.UserID)
		}

		return nil, err
	}

	return s.Get(created.ID)
}

// Find retrieves every live session whose key field equals value.
func (s *Store) Find(key string, value string) ([]*models.Session, error) {
	if err := requireLookup(key); err != nil {
		return nil, err
	}

	return s.collection.Finder().Filter(liveFilter(bson.D{{Key: key, Value: value}})).Find(*s.ctx)
}

// FindFirst retrieves the first live session whose key field equals value.
func (s *Store) FindFirst(key string, value string) (*models.Session, error) {
	if err := requireLookup(key); err != nil {
		return nil, err
	}

	return s.findOne(bson.D{{Key: key, Value: value}}, fmt.Sprintf("no session with %s %q", key, value))
}

// Get retrieves the live session with this id.
func (s *Store) Get(id string) (*models.Session, error) {
	return s.findOne(bson.D{{Key: "_id", Value: id}}, fmt.Sprintf("session %q", id))
}

// GetByToken retrieves a live session by its unguessable bearer token.
func (s *Store) GetByToken(token string) (*models.Session, error) {
	if err := requireToken(token); err != nil {
		return nil, err
	}

	return s.findOne(bson.D{{Key: "token", Value: token}}, "session for token")
}

// Delete removes a session, invalidating its bearer token so it can no longer
// be used to reconnect. Deleting one that is already gone is not an error.
func (s *Store) Delete(id string) error {
	_, err := s.collection.Collection().DeleteOne(*s.ctx, bson.D{{Key: "_id", Value: id}})

	return err
}

// Exists reports whether a live session has this id.
func (s *Store) Exists(id string) (bool, error) {
	count, err := s.collection.Finder().Filter(liveFilter(bson.D{{Key: "_id", Value: id}})).Count(*s.ctx)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// LeaveGame takes a live session out of gameId, if it is still in it.
func (s *Store) LeaveGame(id string, gameId string) error {
	_, err := s.collection.Collection().UpdateOne(
		*s.ctx,
		liveFilter(bson.D{{Key: "_id", Value: id}, {Key: "gameId", Value: gameId}}),
		bson.D{
			{Key: "$unset", Value: bson.D{{Key: "gameId", Value: ""}, {Key: "teamId", Value: ""}, {Key: "slotId", Value: ""}}},
			{Key: "$set", Value: bson.D{{Key: "updatedAt", Value: time.Now()}}},
		},
	)
	if err != nil {
		return fmt.Errorf("taking session %q out of game %q: %w", id, gameId, err)
	}

	return nil
}

// Update sets the update's non-empty fields on a live session; an empty field
// is left as it is rather than cleared.
func (s *Store) Update(id string, u *models.UpdateSession) error {
	set := bson.D{{Key: "updatedAt", Value: time.Now()}}

	for _, field := range []bson.E{
		{Key: "gameId", Value: u.GameID},
		{Key: "userId", Value: u.UserID},
		{Key: "teamId", Value: u.TeamID},
		{Key: "slotId", Value: u.SlotID},
	} {
		if field.Value != "" {
			set = append(set, field)
		}
	}

	result, err := s.collection.Collection().UpdateOne(
		*s.ctx,
		liveFilter(bson.D{{Key: "_id", Value: id}}),
		bson.D{{Key: "$set", Value: set}},
	)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return fmt.Errorf("user %q already has a session: %w", u.UserID, repoErrors.ErrDuplicate)
		}

		return err
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("session %q: %w", id, repoErrors.ErrNotFound)
	}

	return nil
}
