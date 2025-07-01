package game

import (
	"context"
	"errors"
	"github.com/chenmingyong0423/go-mongox/v2"
	"github.com/chenmingyong0423/go-mongox/v2/builder/query"
	"go.mongodb.org/mongo-driver/v2/bson"
	"indri/internal/clients/mongodb"
	"indri/internal/models"
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

	return &Repo{
		ctx:        &ctx,
		client:     client,
		collection: gameColl,
	}
}

// New creates a new game, given an ID.
func (s *Repo) New(id string, defaultGameData models.Game) (*models.Game, error) {
	matchingGames, _ := s.collection.Finder().Filter(query.Eq("gameId", id)).FindOne(*s.ctx)

	if matchingGames != nil {
		return nil, errors.New("a game with that id already exists")
	}

	defaultGameData.Code = id

	g, err := s.collection.Creator().InsertOne(*s.ctx, &defaultGameData)
	if err != nil {
		return nil, err
	}

	createdGame, err := s.collection.Finder().Filter(query.Id(g.InsertedID)).FindOne(*s.ctx)
	if err != nil {
		return nil, err
	}

	return createdGame, nil
}

// Get retrieves game data for a specific game ID.
func (s *Repo) Get(id string) (*models.Game, error) {
	return s.collection.Finder().Filter(query.Eq("gameId", id)).FindOne(*s.ctx)
}

// Exists checks to see if a game with the given ID already exists.
func (s *Repo) Exists(id string) (bool, error) {
	count, err := s.collection.Finder().Filter(query.Eq("gameId", id)).Count(*s.ctx)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// Update saves game data to the repository.
func (s *Repo) Update(game *models.Game) error {
	g, err := s.collection.Collection().ReplaceOne(context.Background(), bson.M{"gameId": game.Code}, game)
	if err != nil {
		panic(err)
	}

	if g == nil {
		panic("game not exists")
	}

	return nil
}
