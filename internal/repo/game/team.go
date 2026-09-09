package game

import (
	"fmt"
	"slices"

	"github.com/robbiebyrd/indri/internal/models"
	sessionUtils "github.com/robbiebyrd/indri/internal/utils/session"
)

// HasPlayerOnTeam determines if a given userId is in a game and on a given team.
func (s *Store) HasPlayerOnTeam(id string, teamId string, userId string) bool {
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

// ChangePlayerTeam moves a player to a different team atomically, so a failure
// can't leave the player on no team.
func (s *Store) ChangePlayerTeam(id string, teamId string, userId string) error {
	if err := sessionUtils.ValidateGameAndUser(id, userId); err != nil {
		return err
	}

	return s.Mutate(id, func(g *models.Game) error {
		if _, ok := g.Players[userId]; !ok {
			return fmt.Errorf("player with id %s is not in this game", userId)
		}

		removePlayerFromTeams(g, userId)
		addPlayerToTeam(g, teamId, userId)

		return nil
	})
}

// AddPlayerToTeam adds a player to a team.
func (s *Store) AddPlayerToTeam(id string, teamId string, userId string) error {
	if err := sessionUtils.ValidateGameAndUser(id, userId); err != nil {
		return err
	}

	return s.Mutate(id, func(g *models.Game) error {
		if _, ok := g.Players[userId]; !ok {
			return fmt.Errorf("player with id %s is not in this game", userId)
		}

		for tId, t := range g.Teams {
			if slices.Contains(t.PlayerIDs, userId) {
				return fmt.Errorf("player with id %s is already in team %v", userId, tId)
			}
		}

		addPlayerToTeam(g, teamId, userId)

		return nil
	})
}

// RemovePlayerFromTeam removes a player from any assigned teams in a given game.
func (s *Store) RemovePlayerFromTeam(id string, userId string) error {
	if err := sessionUtils.ValidateGameAndUser(id, userId); err != nil {
		return err
	}

	return s.Mutate(id, func(g *models.Game) error {
		if !removePlayerFromTeams(g, userId) {
			return errAbortMutation
		}

		return nil
	})
}

// addPlayerToTeam appends userId to the given team on the in-memory game,
// creating the teams map if needed.
func addPlayerToTeam(g *models.Game, teamId string, userId string) {
	if g.Teams == nil {
		g.Teams = map[string]models.Team{}
	}

	team := g.Teams[teamId]
	team.PlayerIDs = append(team.PlayerIDs, userId)
	g.Teams[teamId] = team
}

// PlayerOnWhichTeam gets the current team a player is on.
func (s *Store) PlayerOnWhichTeam(id string, userId string) (*string, error) {
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
		return nil, fmt.Errorf("team with id %v does not exists", id)
	}

	return &teamId, nil
}
