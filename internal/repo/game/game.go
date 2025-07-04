package game

import (
	"context"
	"fmt"
	"github.com/chenmingyong0423/go-mongox/v2"
	"github.com/chenmingyong0423/go-mongox/v2/builder/query"
	"github.com/robbiebyrd/indri/internal/clients/mongodb"
	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/repo/utils"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type Repo struct {
	ctx        *context.Context
	collection *mongox.Collection[models.Game]
	client     *mongodb.Client
}

// NewRepo creates a new repository for accessing game data.
func NewRepo() *Repo {
	client, err := mongodb.New()
	if err != nil {
		panic(err)
	}

	gameColl := mongox.NewCollection[models.Game](client.Database, "games")
	ctx := context.Background()

	indexModel := mongo.IndexModel{
		Keys:    bson.M{"code": 1}, // Ascending index on 'email'
		Options: options.Index().SetUnique(true),
	}

	_, err = gameColl.Collection().Indexes().CreateOne(ctx, indexModel)
	if err != nil {
		panic(err)
	}

	return &Repo{
		ctx:        &ctx,
		client:     client,
		collection: gameColl,
	}
}

// New creates a new game, given an ID.
func (s *Repo) New(gameData models.CreateGame) (*models.Game, error) {
	doc, err := utils.CreateBSONDoc(gameData)
	if err != nil {
		return nil, err
	}

	// Create the update document, specifying the fields to update. `nil` fields are not updated,
	// as they are dropped in the conversion. We specify a filter for the requested user ID, so only
	// one document should ever be updated.
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
		fmt.Println("HERE")
		return nil, err
	}

	return s.collection.Finder().Filter(query.Id(objectId)).FindOne(*s.ctx)
}

// FindByCode retrieves game data by its game code.
func (s *Repo) FindByCode(gameCode string) (*models.Game, error) {
	return s.collection.Finder().Filter(query.Eq("code", gameCode)).FindOne(*s.ctx)
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
	// Convert the hex-based, string id we get to an actual ObjectID
	objectId, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return err
	}

	// Unmarshall the bytes to a BSON Document type
	doc, err := utils.CreateBSONDoc(game)
	if err != nil {
		return err
	}

	fmt.Println("doc")
	fmt.Println(doc)
	fmt.Println("doc")

	// Create the update document, specifying the fields to update. `nil` fields are not updated,
	// as they are dropped in the conversion. We specify a filter for the requested user ID, so only
	// one document should ever be updated.
	result, err := s.collection.Collection().UpdateOne(
		context.TODO(),
		bson.D{{Key: "_id", Value: objectId}},
		bson.D{{Key: "$set", Value: doc}},
	)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("user with id %v does not exists", objectId)
	}

	return nil
}

// UpdateField
func (s *Repo) UpdateField(id string, key string, value interface{}) error {
	// Convert the hex-based, string id we get to an actual ObjectID
	objectId, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return err
	}

	// Create the update document, specifying the fields to update. `nil` fields are not updated,
	// as they are dropped in the conversion. We specify a filter for the requested user ID, so only
	// one document should ever be updated.
	result, err := s.collection.Collection().UpdateOne(
		*s.ctx,
		bson.D{{Key: "_id", Value: objectId}},
		bson.D{{Key: "$set", Value: bson.D{{Key: key, Value: value}}}},
	)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("user with id %v does not exists", objectId)
	}

	return nil
}

// DeleteField
func (s *Repo) DeleteField(id string, key string) error {
	// Convert the hex-based, string id we get to an actual ObjectID
	objectId, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return err
	}

	// Create the update document, specifying the fields to update. `nil` fields are not updated,
	// as they are dropped in the conversion. We specify a filter for the requested user ID, so only
	// one document should ever be updated.
	result, err := s.collection.Collection().UpdateOne(
		*s.ctx,
		bson.D{{Key: "_id", Value: objectId}},
		bson.D{{Key: "$unset", Value: bson.D{{Key: key, Value: ""}}}},
	)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("field %v does not exists", key)
	}

	return nil
}
