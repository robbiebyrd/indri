package game

import (
	"context"
	"fmt"
	"github.com/chenmingyong0423/go-mongox/v2"
	"github.com/chenmingyong0423/go-mongox/v2/builder/query"
	"github.com/robbiebyrd/indri/internal/clients/mongodb"
	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/repo/utils"
	"github.com/robbiebyrd/indri/internal/services/user"
	sessionUtils "github.com/robbiebyrd/indri/internal/utils/session"
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

// GetCode returns the game code for a given game id.
func (s *Repo) GetCode(gameCode string) (*string, error) {
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

	doc, err := utils.CreateBSONDoc(game)
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
		return fmt.Errorf("user with id %v does not exists", id)
	}

	return nil
}

// UpdateField updates a field in game.
func (s *Repo) UpdateField(id string, key string, value interface{}) error {
	filterDoc, err := s.getBsonDocForID(id)
	if err != nil {
		return err
	}

	result, err := s.collection.Collection().UpdateOne(
		*s.ctx,
		bson.D{{Key: "_id", Value: filterDoc}},
		bson.D{{Key: "$set", Value: bson.D{{Key: key, Value: value}}}},
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

func (s *Repo) getBsonDocForID(id string) (bson.D, error) {
	objectId, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}

	return bson.D{{Key: "_id", Value: objectId}}, nil
}

// HasPlayer determines if a given userId is in a game.
func (s *Repo) HasPlayer(id string, teamId string, userId string) bool {
	g, err := s.Get(id)
	if err != nil {
		return false
	}

	_, playerExists := g.Teams[teamId].Players[userId]

	return playerExists
}

// AddPlayer adds a player to the game.
func (s *Repo) AddPlayer(id string, teamId string, userId string) error {
	us := user.NewService()

	err := sessionUtils.ValidateStandardKeys(id, teamId, userId)
	if err != nil {
		return err
	}

	g, err := s.Get(id)
	if err != nil {
		return fmt.Errorf("failed retrieving game with id %v", id)
	}

	if s.HasPlayer(id, teamId, userId) {
		return nil
	}

	if g.Teams == nil {
		g.Teams = map[string]models.Team{}
	}

	team, ok := g.Teams[teamId]
	if !ok {
		team = models.Team{Name: teamId}
	}

	if team.Players == nil {
		team.Players = make(map[string]models.Player)
	}

	thisUser, err := us.Get(userId)
	if err != nil {
		return fmt.Errorf("failed retrieving user with userId %v", userId)
	}

	name := thisUser.Name
	if thisUser.DisplayName != nil && *thisUser.DisplayName != "" {
		name = *thisUser.DisplayName
	}

	team.Players[userId] = models.Player{
		Name:         name,
		Host:         !s.HasHost(id),
		Disconnected: false,
	}

	g.Teams[teamId] = team

	if err = s.Update(id, &models.UpdateGame{Teams: &g.Teams}); err != nil {
		return err
	}

	return nil
}

// RemovePlayer removes a player from a game.
func (s *Repo) RemovePlayer(id string, teamId string, userId string) error {
	err := sessionUtils.ValidateStandardKeys(id, teamId, userId)
	if err != nil {
		return err
	}

	g, err := s.Get(id)
	if err != nil {
		return fmt.Errorf("failed retrieving game with id %v", id)
	}

	team := g.Teams[teamId]
	delete(team.Players, userId)
	g.Teams[teamId] = team

	return nil
}

// ConnectPlayer marks the player as offline.
func (s *Repo) ConnectPlayer(id string, teamId string, userId string) error {
	return s.markPlayerConnected(id, teamId, userId, true)
}

// DisconnectPlayer marks the player as offline.
func (s *Repo) DisconnectPlayer(id string, teamId string, userId string) error {
	return s.markPlayerConnected(id, teamId, userId, false)
}

// HasHost checks to see if the game has a host already.
func (s *Repo) HasHost(id string) bool {
	g, err := s.Get(id)
	if err != nil {
		return false
	}

	for _, team := range g.Teams {
		for _, player := range team.Players {
			if player.Host {
				return true
			}
		}
	}

	return false
}

// PlayerIsHost checks to see if a player is currently the host of the game..
func (s *Repo) PlayerIsHost(id string, playerId string) bool {
	g, err := s.Get(id)
	if err != nil {
		return false
	}

	for _, team := range g.Teams {
		for _, player := range team.Players {
			if player.Host {
				return true
			}
		}
	}

	return false
}

// markPlayerConnected marks the player's connected status.
func (s *Repo) markPlayerConnected(
	id string,
	teamId string,
	userId string,
	connected bool,
) error {
	g, err := s.validateKeysAndGetGame(id, teamId, userId)
	if err != nil {
		return err
	}

	player, ok := g.Teams[teamId].Players[userId]
	if !ok {
		return fmt.Errorf("no player found for game %v", id)
	}

	player.Disconnected = !connected
	g.Teams[teamId].Players[userId] = player

	if err = s.Update(id, &models.UpdateGame{Teams: &g.Teams}); err != nil {
		return err
	}

	return nil
}

func (s *Repo) validateKeysAndGetGame(id string, teamId string, userId string) (*models.Game, error) {
	err := sessionUtils.ValidateStandardKeys(id, teamId, userId)
	if err != nil {
		return nil, err
	}

	g, err := s.Get(id)
	if err != nil {
		return nil, fmt.Errorf("failed retrieving game with gameId %v", id)
	}

	return g, nil
}
