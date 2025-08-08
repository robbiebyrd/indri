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
	repoUtils "github.com/robbiebyrd/indri/internal/repo/utils"
)

var collectionName = "session"

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
			Keys: bson.D{
				{"userId", 1},
			},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{
				{"userId", 1},
				{"gameId", 1},
			},
		},
		{
			Keys: bson.D{
				{"gameId", 1},
				{"userId", 1},
				{"teamId", 1},
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

// New creates a new user, given an ID.
func (s *Store) New(createSession models.CreateSession) (*models.Session, error) {
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

// Find retrieves user data records for a specific key/value.
func (s *Store) Find(key string, value string) ([]*models.Session, error) {
	return s.collection.Finder().Filter(query.Eq(key, value)).Find(*s.ctx)
}

// FindFirst retrieves the first user data record, given a key/value.
func (s *Store) FindFirst(key string, value string) (*models.Session, error) {
	return s.collection.Finder().Filter(query.Eq(key, value)).FindOne(*s.ctx)
}

// Get retrieves user data for a specific user ID.
func (s *Store) Get(id string) (*models.Session, error) {
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
func (s *Store) Update(sessionId string, session *models.UpdateSession) error {
	session.UpdatedAt = time.Now()
	objectId, err := bson.ObjectIDFromHex(sessionId)
	if err != nil {
		return err
	}

	doc, err := repoUtils.CreateBSONDoc(session)
	if err != nil {
		return err
	}

	// Create the update document, specifying the fields to update. `nil` fields are not updated,
	// as they are dropped in the conversion. We specify a filter for the requested user ID, so only
	// one document should ever be updated.
	result, err := s.collection.Collection().UpdateOne(
		*s.ctx,
		bson.D{{Key: "_id", Value: objectId}},
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

func (s *Store) createNewSession(session models.CreateSession) (*models.Session, error) {
	session.CreatedAt = time.Now()

	doc, err := repoUtils.CreateBSONDoc(session)
	if err != nil {
		return nil, err
	}

	result, err := s.collection.Collection().InsertOne(*s.ctx, &doc)
	if err != nil {
		return nil, err
	}

	insertedId := result.InsertedID.(bson.ObjectID).Hex()

	return s.Get(insertedId)
}

func (s *Store) isSessionInGameAndTeam(gameId, teamId, sessionGameId, sessionTeamId string) bool {
	return gameId == sessionGameId || teamId == sessionTeamId
}

func (s *Store) isSessionInGame(gameId, sessionGameId string) bool {
	return gameId == sessionGameId
}
