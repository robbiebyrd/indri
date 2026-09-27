package session

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
	"github.com/robbiebyrd/indri/internal/repo/ids"
	repoUtils "github.com/robbiebyrd/indri/internal/repo/utils"
)

var collectionName = "session"

// sessionMaxAge is an absolute cap on how long a session (and its bearer token)
// remains valid, enforced by a TTL index as defense in depth: even a session
// that is never explicitly logged out expires and its token stops working.
const sessionMaxAge = 7 * 24 * time.Hour

type MongoStore struct {
	ctx        *context.Context
	collection *mongox.Collection[models.Session]
	client     *mongodb.Client
}

// NewMongoStore creates a new repository for accessing session data.
func NewMongoStore(ctx context.Context, client *mongodb.Client) (*MongoStore, error) {
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

	return &MongoStore{
		ctx:        &ctx,
		client:     client,
		collection: sessionColl,
	}, nil
}

// New returns an existing session for the user if one exists, or creates a new one.
func (s *MongoStore) New(createSession models.CreateSession) (*models.Session, error) {
	if createSession.UserID == "" {
		return nil, fmt.Errorf("session must have a user id")
	}

	matchingSession, _ := s.collection.Finder().Filter(
		query.Eq("userId", createSession.UserID),
	).FindOne(*s.ctx)

	switch matchingSession {
	case nil:
		return s.createNewSession(createSession)
	default:
		return matchingSession, nil
	}
}

// Find retrieves session records for a specific key/value.
func (s *MongoStore) Find(key string, value string) ([]*models.Session, error) {
	return s.collection.Finder().Filter(query.Eq(key, value)).Find(*s.ctx)
}

// FindFirst retrieves the first session record, given a key/value.
func (s *MongoStore) FindFirst(key string, value string) (*models.Session, error) {
	return s.collection.Finder().Filter(query.Eq(key, value)).FindOne(*s.ctx)
}

// Get retrieves session data for a specific session ID.
func (s *MongoStore) Get(id string) (*models.Session, error) {
	return s.collection.Finder().Filter(bson.D{{Key: "_id", Value: id}}).FindOne(*s.ctx)
}

// GetByToken retrieves a session by its unguessable bearer token.
func (s *MongoStore) GetByToken(token string) (*models.Session, error) {
	if token == "" {
		return nil, fmt.Errorf("token is empty")
	}

	return s.collection.Finder().Filter(query.Eq("token", token)).FindOne(*s.ctx)
}

// Delete removes a session, invalidating its bearer token so it can no longer
// be used to reconnect. It is idempotent: deleting an already-gone session is
// not an error.
func (s *MongoStore) Delete(id string) error {
	_, err := s.collection.Collection().DeleteOne(*s.ctx, bson.D{{Key: "_id", Value: id}})
	return err
}

// Exists checks to see if a session with the given ID already exists.
func (s *MongoStore) Exists(id string) (bool, error) {
	count, err := s.collection.Finder().Filter(bson.D{{Key: "_id", Value: id}}).Count(*s.ctx)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// Update saves session data to the repository.
func (s *MongoStore) Update(sessionId string, session *models.UpdateSession) error {
	session.UpdatedAt = time.Now()

	doc, err := repoUtils.CreateBSONDoc(session)
	if err != nil {
		return err
	}

	result, err := s.collection.Collection().UpdateOne(
		*s.ctx,
		bson.D{{Key: "_id", Value: sessionId}},
		bson.D{{Key: "$set", Value: doc}},
	)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("session with id %v does not exists", sessionId)
	}

	return nil
}

func (s *MongoStore) createNewSession(session models.CreateSession) (*models.Session, error) {
	now := time.Now()
	newSession := &models.Session{
		ID:        ids.New(),
		Token:     session.Token,
		UserID:    &session.UserID,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if session.GameID != "" {
		newSession.GameID = &session.GameID
	}
	if session.TeamID != "" {
		newSession.TeamID = &session.TeamID
	}

	doc, err := repoUtils.CreateBSONDoc(newSession)
	if err != nil {
		return nil, err
	}

	if _, err := s.collection.Collection().InsertOne(*s.ctx, &doc); err != nil {
		// Lost a race with a concurrent login for the same user (the unique
		// userId index rejected the insert). Return the winner's session
		// rather than a raw duplicate-key error — one session per user.
		if mongo.IsDuplicateKeyError(err) {
			return s.FindFirst("userId", session.UserID)
		}

		return nil, err
	}

	return s.Get(newSession.ID)
}

func (s *MongoStore) isSessionInGameAndTeam(gameId, teamId, sessionGameId, sessionTeamId string) bool {
	return gameId == sessionGameId || teamId == sessionTeamId
}

func (s *MongoStore) isSessionInGame(gameId, sessionGameId string) bool {
	return gameId == sessionGameId
}
