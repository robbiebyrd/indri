package game

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
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

var collectionName = "game"

type Store struct {
	ctx        *context.Context
	collection *mongox.Collection[models.Game]
	client     *mongodb.Client
}

// NewStore creates a new repository for accessing game data.
func NewStore(ctx context.Context, client *mongodb.Client) (*Store, error) {
	gameColl := mongox.NewCollection[models.Game](client.Database, collectionName)

	indexModels := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "code", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
	}

	_, err := gameColl.Collection().Indexes().CreateMany(ctx, indexModels)
	if err != nil {
		return nil, err
	}

	return &Store{
		ctx:        &ctx,
		client:     client,
		collection: gameColl,
	}, nil
}

// New creates a new game, given an ID.
func (s *Store) New(code string, script *models.Script, privateGame bool) (*models.Game, error) {
	gameDataModel := models.CreateGame{
		Code:      code,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Private:   privateGame,
	}

	if script != nil {
		gameDataModel.Teams = &script.Teams
		gameDataModel.Stage = &script.Stage
		gameDataModel.PublicData = script.PublicData
		gameDataModel.PrivateData = script.PrivateData
	}

	doc, err := repoUtils.CreateBSONDoc(gameDataModel)
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

// Get retrieves game data for a specific game ID.
func (s *Store) Get(id string) (*models.Game, error) {
	objectId, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}

	return s.collection.Finder().Filter(query.Id(objectId)).FindOne(*s.ctx)
}

// FindByCode retrieves game data by its game code.
func (s *Store) FindByCode(gameCode string) (*models.Game, error) {
	return s.collection.Finder().Filter(query.Eq("code", gameCode)).FindOne(*s.ctx)
}

// FindOpen retrieves game data by its game code.
func (s *Store) FindOpen(limit int) ([]*models.Game, error) {
	return s.collection.Finder().Filter(query.Ne("private", true)).Limit(int64(limit)).Find(*s.ctx)
}

// GetIDHex returns the game code for a given game id.
func (s *Store) GetIDHex(gameCode string) (*string, error) {
	retrievedGame, err := s.FindByCode(gameCode)
	if err != nil {
		return nil, err
	}

	gameId := retrievedGame.ID.Hex()

	return &gameId, nil
}

// Exists checks to see if a game with the given ID already exists.
func (s *Store) Exists(id string) (bool, error) {
	count, err := s.collection.Finder().Filter(query.Eq("code", id)).Count(*s.ctx)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// Update saves game data to the repository.
func (s *Store) Update(id string, game *models.UpdateGame) error {
	filterDoc, err := s.getBsonDocForID(id)
	if err != nil {
		return err
	}

	game.UpdatedAt = time.Now()

	doc, err := repoUtils.CreateBSONDoc(game)
	if err != nil {
		return err
	}

	result, err := s.collection.Collection().UpdateOne(
		context.TODO(),
		filterDoc,
		bson.D{{Key: "$set", Value: doc}},
	)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("error updating game: game with id %v does not exists", id)
	}

	return nil
}

// UpdateField updates a field in the game.
func (s *Store) UpdateField(id string, key string, value interface{}) error {
	filterDoc, err := s.getBsonDocForID(id)
	if err != nil {
		return err
	}

	result, err := s.collection.Collection().UpdateOne(
		*s.ctx,
		filterDoc,
		bson.D{
			{Key: "$set", Value: bson.D{
				{Key: key, Value: value},
				{Key: "updatedAt", Value: time.Now()},
			}},
			{Key: "$inc", Value: bson.D{{Key: "version", Value: 1}}},
		},
	)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("error updating game field: game with id %v does not exists", id)
	}

	return nil
}

// DeleteField removes a field from a game.
func (s *Store) DeleteField(id string, key string) error {
	filterDoc, err := s.getBsonDocForID(id)
	if err != nil {
		return err
	}

	result, err := s.collection.Collection().UpdateOne(
		*s.ctx,
		filterDoc,
		bson.D{
			{Key: "$unset", Value: bson.D{
				{Key: key, Value: ""},
			}},
			{Key: "$set", Value: bson.D{
				{Key: "updatedAt", Value: time.Now()},
			}},
			{Key: "$inc", Value: bson.D{{Key: "version", Value: 1}}},
		},
	)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("field %v does not exists", key)
	}

	return nil
}

const maxMutateRetries = 50

// errAbortMutation lets an apply function abort a mutate without writing and
// without surfacing an error (for no-op cases).
var errAbortMutation = errors.New("mutation aborted")

// Mutate applies apply to a freshly-loaded game and persists it under an
// optimistic-concurrency check on the version field, reloading and retrying
// if another writer committed in between. This keeps a read-modify-write
// sequence atomic against concurrent writers using only a version column, so
// the guarantee is portable across database backends.
func (s *Store) Mutate(id string, apply func(g *models.Game) error) error {
	objectId, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return err
	}

	for attempt := 0; attempt < maxMutateRetries; attempt++ {
		g, err := s.Get(id)
		if err != nil {
			return err
		}

		expectedVersion := g.Version

		if err := apply(g); err != nil {
			if errors.Is(err, errAbortMutation) {
				return nil
			}

			return err
		}

		g.UpdatedAt = time.Now()
		g.Version = expectedVersion + 1

		doc, err := repoUtils.CreateBSONDoc(g)
		if err != nil {
			return err
		}

		result, err := s.collection.Collection().UpdateOne(
			*s.ctx,
			bson.D{{Key: "_id", Value: objectId}, {Key: "version", Value: expectedVersion}},
			bson.D{{Key: "$set", Value: withoutKey(doc, "_id")}},
		)
		if err != nil {
			return err
		}

		if result.MatchedCount == 1 {
			return nil
		}

		// The version moved under us; back off a jittered interval to let the
		// contending writers spread out, then reload and retry.
		time.Sleep(time.Duration(rand.Intn(5*(attempt+1))) * time.Millisecond)
	}

	return fmt.Errorf("could not update game %v after %d attempts due to concurrent modification", id, maxMutateRetries)
}

// withoutKey returns a copy of doc with the given top-level key removed, used
// to keep the immutable _id out of a $set.
func withoutKey(doc bson.D, key string) bson.D {
	out := make(bson.D, 0, len(doc))

	for _, e := range doc {
		if e.Key != key {
			out = append(out, e)
		}
	}

	return out
}

func (s *Store) getBsonDocForID(id string) (bson.D, error) {
	objectId, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}

	return bson.D{{Key: "_id", Value: objectId}}, nil
}
