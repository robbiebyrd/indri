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
)

var collectionName = "game"

type Repo struct {
	ctx        *context.Context
	collection *mongox.Collection[models.Game]
	client     *mongodb.Client
}

// NewRepo creates a new repository for accessing game data.
func NewRepo(ctx context.Context, client *mongodb.Client) (*Repo, error) {
	gameColl := mongox.NewCollection[models.Game](client.Database, collectionName)

	indexModel := mongo.IndexModel{
		Keys:    bson.M{"code": 1}, // Ascending index on 'email'
		Options: options.Index().SetUnique(true),
	}

	_, err := gameColl.Collection().Indexes().CreateOne(ctx, indexModel)
	if err != nil {
		return nil, err
	}

	return &Repo{
		ctx:        &ctx,
		client:     client,
		collection: gameColl,
	}, nil

}

// New creates a new game, given an ID.
func (s *Repo) New(code string, script *models.Script) (*models.Game, error) {
	gameDataModel := models.CreateGame{
		Code:      code,
		CreatedAt: time.Now(),
	}

	if script != nil {
		gameDataModel.Teams = &script.DefaultTeams
		gameDataModel.Stage = &script.DefaultStage
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
func (s *Repo) Get(id string) (*models.Game, error) {
	objectId, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}

	return s.collection.Finder().Filter(query.Id(objectId)).FindOne(*s.ctx)
}

// FindByCode retrieves game data by its game code.
func (s *Repo) FindByCode(gameCode string) (*models.Game, error) {
	return s.collection.Finder().Filter(query.Eq("code", gameCode)).FindOne(*s.ctx)
}

// GetIDHex returns the game code for a given game id.
func (s *Repo) GetIDHex(gameCode string) (*string, error) {
	retrievedGame, err := s.FindByCode(gameCode)
	if err != nil {
		return nil, err
	}

	gameId := retrievedGame.ID.Hex()

	return &gameId, nil
}

// Exists checks to see if a game with the given ID already exists.
func (s *Repo) Exists(id string) (bool, error) {
	count, err := s.collection.Finder().Filter(query.Eq("code", id)).Count(*s.ctx)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// Update saves game data to the repository.
func (s *Repo) Update(id string, game *models.UpdateGame) error {
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
		return fmt.Errorf("game with id %v does not exists", id)
	}

	return nil
}

// UpdateField updates a field in the game.
func (s *Repo) UpdateField(id string, key string, value interface{}) error {
	filterDoc, err := s.getBsonDocForID(id)
	if err != nil {
		return err
	}

	result, err := s.collection.Collection().UpdateOne(
		*s.ctx,
		bson.D{{Key: "_id", Value: filterDoc}},
		bson.D{
			{Key: "$set", Value: bson.D{
				{Key: key, Value: value},
				{Key: "UpdatedAt", Value: time.Now()},
			}},
		},
	)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("user with id %v does not exists", id)
	}

	return nil
}

// DeleteField removes a field from a game.
func (s *Repo) DeleteField(id string, key string) error {
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
				{Key: "UpdatedAt", Value: time.Now()},
			}},
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

func (s *Repo) getBsonDocForID(id string) (bson.D, error) {
	objectId, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}

	return bson.D{{Key: "_id", Value: objectId}}, nil
}
