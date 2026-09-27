package game

import (
	"fmt"

	"github.com/robbiebyrd/indri/internal/models"
	repoErrors "github.com/robbiebyrd/indri/internal/repo"
)

func (s *PostgresStore) AddPlayer(id string, userId string, displayName string) error {
	return s.Mutate(id, func(g *models.Game) error {
		if _, exists := g.Players[userId]; exists {
			return fmt.Errorf("player %q: %w", userId, repoErrors.ErrDuplicate)
		}
		if g.Players == nil {
			g.Players = map[string]models.Player{}
		}
		g.Players[userId] = models.Player{
			Name:      displayName,
			Connected: false,
		}
		return nil
	})
}

func (s *PostgresStore) RemovePlayer(id string, userId string) error {
	return s.Mutate(id, func(g *models.Game) error {
		delete(g.Players, userId)
		return nil
	})
}

func (s *PostgresStore) HasPlayer(id string, userId string) bool {
	g, err := s.Get(id)
	if err != nil {
		return false
	}
	_, has := g.Players[userId]
	return has
}

func (s *PostgresStore) PlayerOnATeam(id string, userId string) bool {
	g, err := s.Get(id)
	if err != nil {
		return false
	}
	for _, team := range g.Teams {
		for _, pid := range team.PlayerIDs {
			if pid == userId {
				return true
			}
		}
	}
	return false
}

func (s *PostgresStore) ConnectPlayer(id string, userId string) error {
	return s.Mutate(id, func(g *models.Game) error {
		p, ok := g.Players[userId]
		if !ok {
			return fmt.Errorf("player %q: %w", userId, repoErrors.ErrNotFound)
		}
		p.Connected = true
		g.Players[userId] = p
		return nil
	})
}

func (s *PostgresStore) DisconnectPlayer(id string, userId string) error {
	return s.Mutate(id, func(g *models.Game) error {
		p, ok := g.Players[userId]
		if !ok {
			return fmt.Errorf("player %q: %w", userId, repoErrors.ErrNotFound)
		}
		p.Connected = false
		g.Players[userId] = p
		return nil
	})
}
