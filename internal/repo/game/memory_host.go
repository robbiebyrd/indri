package game

import (
	"fmt"

	"github.com/robbiebyrd/indri/internal/models"
	repoErrors "github.com/robbiebyrd/indri/internal/repo"
)

func (s *MemoryStore) HasHost(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	g, ok := s.games[id]
	if !ok {
		return false
	}
	for _, p := range g.Players {
		if p.Host {
			return true
		}
	}
	return false
}

func (s *MemoryStore) PlayerIsHost(id string, playerId string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	g, ok := s.games[id]
	if !ok {
		return false
	}
	p, ok := g.Players[playerId]
	return ok && p.Host
}

func (s *MemoryStore) SetPlayerAsHost(id string, playerId string) error {
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

func (s *MemoryStore) UnsetHost(id string) error {
	return s.Mutate(id, func(g *models.Game) error {
		for uid, p := range g.Players {
			p.Host = false
			g.Players[uid] = p
		}
		return nil
	})
}
