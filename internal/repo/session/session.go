package session

import (
	"context"
	"fmt"
	"time"

	"github.com/chenmingyong0423/go-mongox/v2"
	"github.com/chenmingyong0423/go-mongox/v2/builder/query"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/robbiebyrd/indri/internal/clients/mongodb"
	"github.com/robbiebyrd/indri/internal/models"
	repoUtils "github.com/robbiebyrd/indri/internal/repo/utils"
)

var collectionName = "session"

type Repo struct {
	ctx        *context.Context
	collection *mongox.Collection[models.Session]
	client     *mongodb.Client
}

// NewRepo creates a new repository for accessing user data.
func NewRepo(ctx context.Context, client *mongodb.Client) *Repo {
	sessionColl := mongox.NewCollection[models.Session](client.Database, collectionName)

	return &Repo{
		ctx:        &ctx,
		client:     client,
		collection: sessionColl,
	}
}

// New creates a new user, given an ID.
func (s *Repo) New(session models.CreateSession) (*models.Session, error) {

	if session.UserID == "" || session.GameID == "" || session.TeamID == "" {
		return nil, fmt.Errorf("session must have a user id, game id, team id, and token")
	}

	matchingSession, _ := s.collection.Finder().Filter(
		query.All(
			query.Eq("userId", session.UserID),
		),
	).FindOne(*s.ctx)

	if matchingSession == nil {
		// There is no matching session; create it.
		session.CreatedAt = time.Now()

		doc, err := repoUtils.CreateBSONDoc(session)
		if err != nil {
			return nil, err
		}

		result, err := s.collection.Collection().InsertOne(*s.ctx, &doc)
		if err != nil {
			return nil, err
		}

		// Get the newly inserted ID
		insertedId := result.InsertedID.(bson.ObjectID).Hex()

		// Get the User and return it
		return s.Get(insertedId)
	}

	if isSessionInGameAndTeam(*matchingSession.GameID, *matchingSession.TeamID, session.GameID, session.TeamID) {
		return nil, fmt.Errorf("session already exists for user %v and game id %v, but user is on another team", session.UserID, session.GameID)
	}

	if isSessionInGame(*matchingSession.GameID, session.GameID) {
		return nil, fmt.Errorf("session already exists for user %v but not in game id %v", session.UserID, session.GameID)
	}

	return matchingSession, nil
}

func isSessionInGameAndTeam(gameId, teamId, sessionGameId, sessionTeamId string) bool {
	return gameId == sessionGameId || teamId == sessionTeamId
}

func isSessionInGame(gameId, sessionGameId string) bool {
	return gameId == sessionGameId
}

// Find retrieves user data records for a specific key/value.
func (s *Repo) Find(key string, value string) ([]*models.Session, error) {
	return s.collection.Finder().Filter(query.Eq(key, value)).Find(*s.ctx)
}

// FindFirst retrieves the first user data record, given a key/value.
func (s *Repo) FindFirst(key string, value string) (*models.Session, error) {
	return s.collection.Finder().Filter(query.Eq(key, value)).FindOne(*s.ctx)
}

// Get retrieves user data for a specific user ID.
func (s *Repo) Get(id string) (*models.Session, error) {
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
func (s *Repo) Update(session *models.UpdateSession) error {
	session.UpdatedAt = time.Now()

	doc, err := repoUtils.CreateBSONDoc(session)
	if err != nil {
		return err
	}

	// Create the update document, specifying the fields to update. `nil` fields are not updated,
	// as they are dropped in the conversion. We specify a filter for the requested user ID, so only
	// one document should ever be updated.
	result, err := s.collection.Collection().UpdateOne(
		*s.ctx,
		bson.D{{Key: "_id", Value: session.ID}},
		bson.D{{Key: "$set", Value: doc}},
	)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("session with id %v does not exists", session.ID)
	}

	return nil
}
