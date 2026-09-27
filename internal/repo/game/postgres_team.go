package game

import (
	"fmt"

	"github.com/robbiebyrd/indri/internal/models"
	repoErrors "github.com/robbiebyrd/indri/internal/repo"
)

func (s *PostgresStore) HasPlayerOnTeam(id string, teamId string, userId string) bool {
	g, err := s.Get(id)
	if err != nil {
		return false
	}
	team, ok := g.Teams[teamId]
	if !ok {
		return false
	}
	return containsID(team.PlayerIDs, userId)
}

func (s *PostgresStore) AddPlayerToTeam(id string, teamId string, userId string) error {
	return s.Mutate(id, func(g *models.Game) error {
		team, ok := g.Teams[teamId]
		if !ok {
			return fmt.Errorf("team %q: %w", teamId, repoErrors.ErrNotFound)
		}
		if _, ok := g.Players[userId]; !ok {
			return fmt.Errorf("player %q: %w", userId, repoErrors.ErrNotFound)
		}
		if !containsID(team.PlayerIDs, userId) {
			team.PlayerIDs = append(team.PlayerIDs, userId)
			g.Teams[teamId] = team
		}
		return nil
	})
}

func (s *PostgresStore) RemovePlayerFromTeam(id string, userId string) error {
	return s.Mutate(id, func(g *models.Game) error {
		for tid, team := range g.Teams {
			if containsID(team.PlayerIDs, userId) {
				team.PlayerIDs = filterOutID(team.PlayerIDs, userId)
				g.Teams[tid] = team
			}
		}
		return nil
	})
}

func (s *PostgresStore) ChangePlayerTeam(id string, teamId string, userId string) error {
	return s.Mutate(id, func(g *models.Game) error {
		newTeam, ok := g.Teams[teamId]
		if !ok {
			return fmt.Errorf("team %q: %w", teamId, repoErrors.ErrNotFound)
		}
		if _, ok := g.Players[userId]; !ok {
			return fmt.Errorf("player %q: %w", userId, repoErrors.ErrNotFound)
		}
		for tid, team := range g.Teams {
			if containsID(team.PlayerIDs, userId) {
				team.PlayerIDs = filterOutID(team.PlayerIDs, userId)
				g.Teams[tid] = team
			}
		}
		newTeam.PlayerIDs = append(newTeam.PlayerIDs, userId)
		g.Teams[teamId] = newTeam
		return nil
	})
}

func (s *PostgresStore) PlayerOnWhichTeam(id string, userId string) (*string, error) {
	g, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	for tid, team := range g.Teams {
		if containsID(team.PlayerIDs, userId) {
			out := tid
			return &out, nil
		}
	}
	return nil, nil
}
