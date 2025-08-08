package game

import (
	"fmt"
	"slices"
	"time"

	goaway "github.com/TwiN/go-away"

	"github.com/robbiebyrd/indri/internal/models"
	sessionUtils "github.com/robbiebyrd/indri/internal/utils/session"
)

// HasPlayer determines if a given userId is in a game.
func (s *Store) HasPlayer(id string, userId string) bool {
	g, err := s.Get(id)
	if err != nil {
		return false
	}

	for uId := range g.Players {
		if uId == userId {
			return true
		}
	}

	return false
}

// PlayerOnATeam determines if a given userId is in a game.
func (s *Store) PlayerOnATeam(id string, userId string) bool {
	if hasPlayer := s.HasPlayer(id, userId); !hasPlayer {
		return false
	}

	g, err := s.Get(id)
	if err != nil {
		return false
	}

	for _, team := range g.Teams {
		if slices.Contains(team.PlayerIDs, userId) {
			return true
		}
	}

	return false
}

// AddPlayer adds a player to the game.
func (s *Store) AddPlayer(id string, userId string, displayName string) error {
	if s.HasPlayer(id, userId) {
		return fmt.Errorf("player with id %v already exists in game %v", userId, id)
	}

	// Get our standard keys from the session; we need all three to add a player to a game.
	err := sessionUtils.ValidateGameAndUser(id, userId)
	if err != nil {
		return err
	}

	g, err := s.Get(id)
	if err != nil {
		return fmt.Errorf("failed retrieving game with id %v", id)
	}

	if g.Teams == nil {
		g.Teams = map[string]models.Team{}
	}

	if g.Players == nil {
		g.Players = map[string]models.Player{}
	}

	g.Players[userId] = models.Player{
		Name:      goaway.Censor(displayName),
		Host:      !s.HasHost(id),
		Connected: false,
	}

	updateGame := &models.UpdateGame{
		Teams:       &g.Teams,
		Players:     &g.Players,
		Stage:       &g.Stage,
		UpdatedAt:   time.Time{},
		PublicData:  g.PublicData,
		PrivateData: g.PrivateData,
		PlayerData:  g.PlayerData,
		Private:     g.Private,
	}

	if err = s.Update(id, updateGame); err != nil {
		return err
	}

	return nil
}

// RemovePlayer removes a player from a game.
func (s *Store) RemovePlayer(id string, userId string) error {
	err := sessionUtils.ValidateGameAndUser(id, userId)
	if err != nil {
		return err
	}

	err = s.RemovePlayerFromTeam(id, userId)
	if err != nil {
		return err
	}

	g, err := s.Get(id)
	if err != nil {
		return fmt.Errorf("failed retrieving game with id %v", id)
	}

	delete(g.Players, userId)

	updateGame := &models.UpdateGame{
		Teams:       &g.Teams,
		Players:     &g.Players,
		Stage:       &g.Stage,
		UpdatedAt:   time.Time{},
		PublicData:  g.PublicData,
		PrivateData: g.PrivateData,
		PlayerData:  g.PlayerData,
		Private:     g.Private,
	}

	if err = s.Update(id, updateGame); err != nil {
		return err
	}

	return nil
}

// ConnectPlayer marks the player as offline.
func (s *Store) ConnectPlayer(id string, userId string) error {
	return s.markPlayerConnected(id, userId, true)
}

// DisconnectPlayer marks the player as offline.
func (s *Store) DisconnectPlayer(id string, userId string) error {
	return s.markPlayerConnected(id, userId, false)
}

// markPlayerConnected marks the player's connected status.
func (s *Store) markPlayerConnected(
	id string,
	userId string,
	connected bool,
) error {
	g, err := s.validateKeysAndGetGame(id, userId)
	if err != nil {
		return err
	}

	_, ok := g.Players[userId]
	if !ok {
		return fmt.Errorf("no player found for game %v", id)
	}

	if err = s.UpdateField(id, "players."+userId+".connected", connected); err != nil {
		return err
	}

	return nil
}

func (s *Store) validateKeysAndGetGame(id string, userId string) (*models.Game, error) {
	err := sessionUtils.ValidateGameAndUser(id, userId)
	if err != nil {
		return nil, err
	}

	g, err := s.Get(id)
	if err != nil {
		return nil, fmt.Errorf("failed retrieving game with gameId %v", id)
	}

	return g, nil
}
