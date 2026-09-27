package game

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
	repoUtils "github.com/robbiebyrd/indri/internal/repo/utils"
	"github.com/robbiebyrd/indri/internal/services/events"
	"github.com/robbiebyrd/indri/internal/services/lock"
	"github.com/robbiebyrd/indri/internal/services/mutation"
)

var collectionName = "game"

type MongoStore struct {
	ctx        *context.Context
	collection *mongox.Collection[models.Game]
	client     *mongodb.Client
	locks      lock.Manager
	changes    changePublisher
}

// NewMongoStore creates a new repository for accessing game data. locks provides the
// cross-instance serialization used by Mutate; publisher receives the change
// deltas computed at each write.
func NewMongoStore(ctx context.Context, client *mongodb.Client, locks lock.Manager, publisher events.Publisher) (*MongoStore, error) {
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

	return &MongoStore{
		ctx:        &ctx,
		client:     client,
		collection: gameColl,
		locks:      locks,
		changes:    changePublisher{ctx: ctx, publisher: publisher},
	}, nil
}

// New creates a new game, given a code.
func (s *MongoStore) New(code string, script *models.Script, privateGame bool) (*models.Game, error) {
	g := newGame(code, script, privateGame)

	doc, err := repoUtils.CreateBSONDoc(g)
	if err != nil {
		return nil, err
	}
	if _, err := s.collection.Collection().InsertOne(*s.ctx, &doc); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil, fmt.Errorf("game with code %s already exists: %w", code, repoErrors.ErrDuplicate)
		}
		return nil, err
	}
	return s.Get(g.ID)
}

// Get retrieves game data for a specific game ID.
func (s *MongoStore) Get(id string) (*models.Game, error) {
	return s.collection.Finder().Filter(bson.D{{Key: "_id", Value: id}}).FindOne(*s.ctx)
}

// FindByCode retrieves game data by its game code.
func (s *MongoStore) FindByCode(gameCode string) (*models.Game, error) {
	return s.collection.Finder().Filter(query.Eq("code", gameCode)).FindOne(*s.ctx)
}

// FindOpen retrieves game data by its game code.
func (s *MongoStore) FindOpen(limit int) ([]*models.Game, error) {
	return s.collection.Finder().Filter(query.Ne("private", true)).Limit(int64(limit)).Find(*s.ctx)
}

// GetIDHex returns the game ID for a given game code. The ID is already a string.
func (s *MongoStore) GetIDHex(gameCode string) (*string, error) {
	g, err := s.FindByCode(gameCode)
	if err != nil {
		return nil, err
	}
	return &g.ID, nil
}

// Exists checks to see if a game with the given ID already exists.
func (s *MongoStore) Exists(id string) (bool, error) {
	count, err := s.collection.Finder().Filter(query.Eq("code", id)).Count(*s.ctx)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// Update saves game data to the repository.
func (s *MongoStore) Update(id string, game *models.UpdateGame) error {
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

// UpdateField sets the value at a dotted JSON path.
func (s *MongoStore) UpdateField(id string, key string, value interface{}) error {
	return updateField(s, id, key, value)
}

// DeleteField removes the value at a dotted JSON path.
func (s *MongoStore) DeleteField(id string, key string) error {
	return deleteField(s, id, key)
}

// Mutate applies apply to the game as a conflict-free read-modify-write. The
// coordination (distributed lock + version fence + retry) lives in the
// mutation package, above this store; this method only supplies how to load
// the game and how to save it conditionally on its version. A different
// backend (SQLite, ...) reuses the same coordinator by implementing just those
// two operations.
func (s *MongoStore) Mutate(id string, apply func(g *models.Game) error) error {
	var before map[string]interface{}

	return mutation.Run(
		*s.ctx,
		s.locks,
		"game:"+id,
		func() (*models.Game, int64, error) {
			g, err := s.Get(id)
			if err != nil {
				return nil, 0, err
			}

			// Snapshot the JSON view before apply mutates the game in place,
			// so the committed delta can be diffed against it.
			before, err = events.ToMap(g)
			if err != nil {
				return nil, 0, err
			}

			return g, g.Version, nil
		},
		apply,
		func(g *models.Game, expectedVersion int64) (bool, error) {
			committed, err := s.saveWithVersion(id, g, expectedVersion)
			if err != nil || !committed {
				return committed, err
			}

			s.changes.diff(id, before, g)

			return true, nil
		},
	)
}

// saveWithVersion persists the whole game only if its stored version still
// equals expectedVersion, bumping the version on success. It reports whether
// the write committed. Callers publish the resulting delta themselves; see
// Mutate.
func (s *MongoStore) saveWithVersion(id string, g *models.Game, expectedVersion int64) (bool, error) {
	g.UpdatedAt = time.Now()
	g.Version = expectedVersion + 1

	doc, err := repoUtils.CreateBSONDoc(g)
	if err != nil {
		return false, err
	}

	result, err := s.collection.Collection().UpdateOne(
		*s.ctx,
		bson.D{{Key: "_id", Value: id}, {Key: "version", Value: expectedVersion}},
		bson.D{{Key: "$set", Value: withoutKey(doc, "_id")}},
	)
	if err != nil {
		return false, err
	}

	return result.MatchedCount == 1, nil
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

func (s *MongoStore) getBsonDocForID(id string) (bson.D, error) {
	return bson.D{{Key: "_id", Value: id}}, nil
}
