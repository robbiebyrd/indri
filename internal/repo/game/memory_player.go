package game

import (
	"fmt"

	"github.com/robbiebyrd/indri/internal/models"
	repoErrors "github.com/robbiebyrd/indri/internal/repo"
)

func (s *MemoryStore) AddPlayer(id string, userId string, displayName string) error {
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

func (s *MemoryStore) RemovePlayer(id string, userId string) error {
	return s.Mutate(id, func(g *models.Game) error {
		delete(g.Players, userId)
		return nil
	})
}

func (s *MemoryStore) HasPlayer(id string, userId string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	g, ok := s.games[id]
	if !ok {
		return false
	}
	_, has := g.Players[userId]
	return has
}

func (s *MemoryStore) PlayerOnATeam(id string, userId string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	g, ok := s.games[id]
	if !ok {
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

func (s *MemoryStore) ConnectPlayer(id string, userId string) error {
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

func (s *MemoryStore) DisconnectPlayer(id string, userId string) error {
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
