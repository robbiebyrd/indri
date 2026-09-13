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
	repoUtils "github.com/robbiebyrd/indri/internal/repo/utils"
	"github.com/robbiebyrd/indri/internal/services/events"
	"github.com/robbiebyrd/indri/internal/services/lock"
)

var collectionName = "game"

// Store is the MongoDB-backed game store. Everything that is not
// Mongo-specific — the mutation bracket, the change deltas and every rule
// built on them — comes from the embedded core; this type supplies only the
// queries.
type Store struct {
	core

	collection *mongox.Collection[models.Game]
	client     *mongodb.Client
}

// NewStore creates a new repository for accessing game data. locks provides the
// cross-instance serialization used by Mutate; publisher receives the change
// deltas computed at each write.
func NewStore(ctx context.Context, client *mongodb.Client, locks lock.Manager, publisher events.Publisher) (*Store, error) {
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
		core: core{
			ctx:       ctx,
			docs:      mongoDocs{collection: gameColl},
			locks:     locks,
			publisher: publisher,
		},
		client:     client,
		collection: gameColl,
	}, nil
}

// mongoDocs is core's persistence port over a MongoDB collection.
type mongoDocs struct {
	collection *mongox.Collection[models.Game]
}

func (m mongoDocs) insert(ctx context.Context, create models.CreateGame) (string, error) {
	doc, err := repoUtils.CreateBSONDoc(create)
	if err != nil {
		return "", err
	}

	result, err := m.collection.Collection().InsertOne(ctx, &doc)
	if err != nil {
		// The unique code index rejected a concurrent create with the same
		// code — surface the friendly error, not a raw duplicate-key.
		if mongo.IsDuplicateKeyError(err) {
			return "", fmt.Errorf("game with code %s already exists", create.Code)
		}

		return "", err
	}

	return result.InsertedID.(bson.ObjectID).Hex(), nil
}

// load reads one game under the caller's context, so a mutation can bound its
// own reads.
func (m mongoDocs) load(ctx context.Context, id string) (*models.Game, error) {
	objectId, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}

	return m.collection.Finder().Filter(query.Id(objectId)).FindOne(ctx)
}

func (m mongoDocs) findByCode(ctx context.Context, gameCode string) (*models.Game, error) {
	return m.collection.Finder().Filter(query.Eq("code", gameCode)).FindOne(ctx)
}

func (m mongoDocs) findOpen(ctx context.Context, limit int) ([]*models.Game, error) {
	return m.collection.Finder().Filter(query.Ne("private", true)).Limit(int64(limit)).Find(ctx)
}

func (m mongoDocs) existsByCode(ctx context.Context, code string) (bool, error) {
	count, err := m.collection.Finder().Filter(query.Eq("code", code)).Count(ctx)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// saveVersioned persists the whole game only if its stored version still
// equals expectedVersion, bumping the version on success. It reports whether
// the write committed. Callers publish the resulting delta themselves; see
// core.Mutate.
func (m mongoDocs) saveVersioned(
	ctx context.Context,
	id string,
	g *models.Game,
	expectedVersion int64,
) (bool, error) {
	objectId, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return false, err
	}

	g.UpdatedAt = time.Now()
	g.Version = expectedVersion + 1

	doc, err := repoUtils.CreateBSONDoc(g)
	if err != nil {
		return false, err
	}

	result, err := m.collection.Collection().UpdateOne(
		ctx,
		bson.D{{Key: "_id", Value: objectId}, {Key: "version", Value: expectedVersion}},
		bson.D{{Key: "$set", Value: withoutKey(doc, "_id")}},
	)
	if err != nil {
		return false, err
	}

	return result.MatchedCount == 1, nil
}

func (m mongoDocs) replace(ctx context.Context, id string, game *models.UpdateGame) error {
	filterDoc, err := bsonDocForID(id)
	if err != nil {
		return err
	}

	doc, err := repoUtils.CreateBSONDoc(game)
	if err != nil {
		return err
	}

	result, err := m.collection.Collection().UpdateOne(
		ctx,
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

func (m mongoDocs) setField(ctx context.Context, id string, key string, value interface{}) error {
	filterDoc, err := bsonDocForID(id)
	if err != nil {
		return err
	}

	result, err := m.collection.Collection().UpdateOne(
		ctx,
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

func (m mongoDocs) unsetField(ctx context.Context, id string, key string) error {
	filterDoc, err := bsonDocForID(id)
	if err != nil {
		return err
	}

	result, err := m.collection.Collection().UpdateOne(
		ctx,
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

// setPlayerConnected sets the player's connected status atomically, only if
// the player still exists, so a concurrent removal cannot recreate a partial
// player document. The version bump keeps it coherent with Mutate's CAS.
func (m mongoDocs) setPlayerConnected(ctx context.Context, id string, userId string, connected bool) error {
	objectId, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return err
	}

	playerKey := playerConnectedKey(userId)

	result, err := m.collection.Collection().UpdateOne(
		ctx,
		bson.D{
			{Key: "_id", Value: objectId},
			{Key: "players." + userId, Value: bson.D{{Key: "$exists", Value: true}}},
		},
		bson.D{
			{Key: "$set", Value: bson.D{
				{Key: playerKey, Value: connected},
				{Key: "updatedAt", Value: time.Now()},
			}},
			{Key: "$inc", Value: bson.D{{Key: "version", Value: 1}}},
		},
	)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("no player %v found in game %v", userId, id)
	}

	return nil
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

func bsonDocForID(id string) (bson.D, error) {
	objectId, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}

	return bson.D{{Key: "_id", Value: objectId}}, nil
}
