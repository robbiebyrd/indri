package game

import (
	"fmt"

	"github.com/robbiebyrd/indri/internal/models"
	repoErrors "github.com/robbiebyrd/indri/internal/repo"
)

func (s *PostgresStore) HasHost(id string) bool {
	g, err := s.Get(id)
	if err != nil {
		return false
	}
	for _, p := range g.Players {
		if p.Host {
			return true
		}
	}
	return false
}

func (s *PostgresStore) PlayerIsHost(id string, playerId string) bool {
	g, err := s.Get(id)
	if err != nil {
		return false
	}
	p, ok := g.Players[playerId]
	return ok && p.Host
}

func (s *PostgresStore) SetPlayerAsHost(id string, playerId string) error {
	return s.Mutate(id, func(g *models.Game) error {
		if _, ok := g.Players[playerId]; !ok {
			return fmt.Errorf("player %q: %w", playerId, repoErrors.ErrNotFound)
		}
		for uid, p := range g.Players {
			p.Host = (uid == playerId)
			g.Players[uid] = p
		}
		return nil
	})
}

func (s *PostgresStore) UnsetHost(id string) error {
	return s.Mutate(id, func(g *models.Game) error {
		for uid, p := range g.Players {
			p.Host = false
			g.Players[uid] = p
		}
		return nil
	})
}
