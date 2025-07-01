package game

import (
	"fmt"
	"github.com/kpechenenko/rword"
	"indri/internal/models"
	"indri/internal/repo/game"
	"indri/internal/services/user"
	sessionUtils "indri/internal/utils/session"
	"log"
	"strings"
)

type Service struct {
	gameRepo *game.Repo
}

var gameService *Service

// NewService creates a new repository for accessing game data.
func NewService() *Service {
	if gameService == nil {
		gameService = &Service{
			gameRepo: game.NewRepo(),
		}
	}

	return gameService
}

// Sanitize removes private items.
func (gs *Service) Sanitize(game *models.Game) *models.Game {
	game.PrivateData = nil

	for i, g := range game.Stage.Scenes {
		g.PrivateData = nil
		game.Stage.Scenes[i] = g
	}

	for i, t := range game.Teams {
		for j, p := range t.Players {
			p.PrivateData = nil
			t.Players[j] = p
		}

		game.Teams[i] = t
	}

	return game
}

// Get will fetch game data for a specific game ID, or create a new one if it doesn't exist.
func (gs *Service) Get(id *string) (*models.Game, error) {
	g, err := gs.gameRepo.Get(*id)
	if err != nil {
		return gs.New(id)
	}

	return g, nil
}

// Fetch retrieves game data for a specific game ID, and returns an error if not found.
func (gs *Service) Fetch(id *string) (*models.Game, error) {
	if id == nil {
		return nil, fmt.Errorf("id is  nil")
	}

	return gs.gameRepo.Get(*id)
}

// Exists checks to see if a game with the given ID already exists.
func (gs *Service) Exists(id *string) bool {
	if id == nil {
		return false
	}

	exists, err := gs.gameRepo.Exists(*id)
	if err != nil {
		return false
	}

	return exists
}

// Update saves game data to the repository.
func (gs *Service) Update(game *models.Game) error {
	err := gs.gameRepo.Update(game)
	if err != nil {
		return err
	}

	return nil
}

// New creates a new game, with or without a Code.
func (gs *Service) New(gameId *string) (*models.Game, error) {
	if gameId == nil || *gameId == "" {
		// If no gameId was given, then we will try to create a random one
		autogenCode, err := gs.getDefaultGameId()
		if err != nil {
			return nil, err
		}

		gameId = autogenCode
	}

	if gs.Exists(gameId) {
		return nil, fmt.Errorf("game with id %s already exists", *gameId)
	}

	log.Printf("creating new game with gameId %v ", *gameId)

	g, err := gs.gameRepo.New(*gameId, models.Game{})
	if err != nil {
		return nil, err
	}

	return g, nil
}

// Reset a game to its defaults.
func (gs *Service) Reset() *models.Game {
	// TODO: reload a game from a script
	return nil
}

// HasPlayer determines if a given userId is in a game.
func (gs *Service) HasPlayer(id *string, teamId *string, userId *string) bool {
	g, err := gs.Get(id)
	if err != nil {
		return false
	}

	_, playerExists := g.Teams[*teamId].Players[*userId]

	return playerExists
}

// HasHost checks to see if the game has a host already.
func (gs *Service) HasHost(id *string) bool {
	g, err := gs.Get(id)
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

// RemovePlayer removes a player from a game.
func (gs *Service) RemovePlayer(gameId *string, teamId *string, userId *string) (*models.Game, error) {
	validGameId, validTeamId, validUserId, err := sessionUtils.ValidateStandardKeys(gameId, teamId, userId)
	if err != nil {
		return nil, err
	}

	g, err := gs.Get(&validGameId)
	if err != nil {
		return nil, fmt.Errorf("failed retrieving game with gameId %v", *gameId)
	}

	team := g.Teams[validTeamId]
	delete(team.Players, validUserId)
	g.Teams[validTeamId] = team

	return g, nil
}

// AddPlayer adds a player to a given game and team.
func (gs *Service) AddPlayer(gameId *string, teamId *string, userId *string) (*models.Game, error) {
	us := user.NewService()

	g, err := gs.Get(gameId)
	if err != nil {
		return nil, fmt.Errorf("failed retrieving game with gameId %v", *gameId)
	}

	if gs.HasPlayer(gameId, teamId, userId) {
		return g, nil
	}

	if g.Teams == nil {
		g.Teams = map[string]models.Team{}
	}

	team, ok := g.Teams[*teamId]
	if !ok {
		team = models.Team{Name: *teamId}
	}

	if team.Players == nil {
		team.Players = make(map[string]models.Player)
	}

	thisUser, err := us.Get(userId)
	if err != nil {
		return nil, fmt.Errorf("failed retrieving user with userId %v", *userId)
	}

	team.Players[*userId] = models.Player{
		Name:         thisUser.DisplayName,
		Host:         !gs.HasHost(gameId),
		Disconnected: false,
	}

	g.Teams[*teamId] = team

	return g, nil
}

// ConnectPlayer marks the player as offline.
func (gs *Service) ConnectPlayer(gameId *string, teamId *string, userId *string) (*models.Game, error) {
	return gs.markPlayerConnected(gameId, teamId, userId, true)
}

// DisconnectPlayer marks the player as offline.
func (gs *Service) DisconnectPlayer(gameId *string, teamId *string, userId *string) (*models.Game, error) {
	return gs.markPlayerConnected(gameId, teamId, userId, false)
}

// DisconnectPlayer marks the player as offline.
func (gs *Service) markPlayerConnected(
	gameId *string,
	teamId *string,
	userId *string,
	connected bool,
) (*models.Game, error) {
	g, err := gs.validateKeysAndGetGame(gameId, teamId, userId)
	if err != nil {
		return nil, err
	}

	player, ok := g.Teams[*teamId].Players[*userId]
	if !ok {
		return nil, fmt.Errorf("no player found for game %v", *gameId)
	}

	player.Disconnected = !connected
	g.Teams[*teamId].Players[*userId] = player

	return g, nil
}

func (gs *Service) validateKeysAndGetGame(gameId *string, teamId *string, userId *string) (*models.Game, error) {
	validGameId, _, _, err := sessionUtils.ValidateStandardKeys(gameId, teamId, userId)
	if err != nil {
		return nil, err
	}

	g, err := gs.Get(&validGameId)
	if err != nil {
		return nil, fmt.Errorf("failed retrieving game with gameId %v", *gameId)
	}

	return g, nil
}

func (gs *Service) makeRandomGameId() (*string, error) {
	g, err := rword.New()
	if err != nil {
		return nil, err
	}

	id := strings.ReplaceAll(g.Str(4)+g.Str(4)+g.Str(4), " ", "-")

	return &id, nil
}

func (gs *Service) getDefaultGameId() (*string, error) {
	autogenCode, _ := gs.makeRandomGameId()
	for gs.Exists(autogenCode) {
		autogenCode, _ = gs.makeRandomGameId()
	}

	return autogenCode, nil
}
