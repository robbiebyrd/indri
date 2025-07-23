package game

import (
	"fmt"
	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/services/user"
	sessionUtils "github.com/robbiebyrd/indri/internal/utils/session"
	"slices"
)

// HasPlayerOnTeam determines if a given userId is in a game and on a given team.
func (s *Repo) HasPlayerOnTeam(id string, teamId string, userId string) bool {
	g, err := s.Get(id)
	if err != nil {
		return false
	}

	_, ok := g.Teams[teamId]
	if !ok {
		return false
	}

	return slices.Contains(g.Teams[teamId].PlayerIDs, userId)
}

// ChangePlayerTeam determines if a given userId is in a game and on a given team.
func (s *Repo) ChangePlayerTeam(id string, teamId string, userId string) error {
	err := s.RemovePlayerFromTeam(id, userId)
	if err != nil {
		return err
	}

	err = s.AddPlayerToTeam(id, teamId, userId)
	if err != nil {
		return err
	}

	return nil
}

// AddPlayerToTeam adds a player to a team.
func (s *Repo) AddPlayerToTeam(id string, teamId string, userId string) error {
	us := user.NewService()

	if !s.HasPlayer(id, userId) {
		return fmt.Errorf("player with id %s is not in this game", id)
	}

	// Get our standard keys from the session; we need all three to add a player to a game.
	if err := sessionUtils.ValidateGameAndUser(id, userId); err != nil {
		return err
	}

	g, err := s.Get(id)
	if err != nil {
		return fmt.Errorf("failed retrieving game with id %v", id)
	}

	_, err = us.Get(userId)
	if err != nil {
		return fmt.Errorf("failed retrieving user with userId %v", userId)
	}

	if g.Teams == nil {
		g.Teams = map[string]models.Team{}
	}

	for tId, t := range g.Teams {
		if slices.Contains(t.PlayerIDs, userId) {
			return fmt.Errorf("player with id %s is already in team %v", id, tId)
		}
	}

	team := g.Teams[teamId]
	team.PlayerIDs = append(team.PlayerIDs, userId)
	g.Teams[teamId] = team

	if err = s.Update(id, &models.UpdateGame{Teams: &g.Teams}); err != nil {
		return err
	}

	return nil
}

// RemovePlayerFromTeam removes a player from any assigned teams in a given game.
func (s *Repo) RemovePlayerFromTeam(id string, userId string) error {
	err := sessionUtils.ValidateGameAndUser(id, userId)
	if err != nil {
		return err
	}

	teamId, err := s.PlayerOnWhichTeam(id, userId)
	if err != nil {
		return err
	}

	g, err := s.Get(id)
	if err != nil {
		return fmt.Errorf("failed retrieving game with id %v", id)
	}

	var modified bool

	var newPlayerIds []string

	for _, pId := range g.Teams[*teamId].PlayerIDs {
		if pId != userId {
			newPlayerIds = append(newPlayerIds, pId)
		} else {
			modified = true
		}
	}

	team := g.Teams[*teamId]
	team.PlayerIDs = newPlayerIds
	g.Teams[*teamId] = team

	if !modified {
		return nil
	}

	if err = s.Update(id, &models.UpdateGame{Teams: &g.Teams}); err != nil { //nolint:wsl
		return err
	}

	return nil
}

// PlayerOnWhichTeam gets the current team a player is on.
func (s *Repo) PlayerOnWhichTeam(id string, userId string) (*string, error) {
	g, err := s.Get(id)
	if err != nil {
		return nil, fmt.Errorf("failed retrieving game with id %v", id)
	}

	var teamId string

	for tId, team := range g.Teams {
		for _, pId := range team.PlayerIDs {
			if pId == userId {
				teamId = tId
			}
		}
	}

	if teamId == "" {
		return nil, fmt.Errorf("user with id %v does not exists", id)
	}

	return &teamId, nil
}
