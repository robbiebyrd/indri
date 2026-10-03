package game

import (
	"fmt"

	"github.com/robbiebyrd/indri/internal/models"
)

// HasHost checks to see if the game has a host already.
func (s *core) HasHost(id string) bool {
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

// PlayerIsHost checks to see if a player is currently the host of the game.
func (s *core) PlayerIsHost(id string, playerId string) bool {
	g, err := s.Get(id)
	if err != nil {
		return false
	}

	return g.Players[playerId].Host
}

// UnsetHost clears the host flag on every player, atomically.
func (s *core) UnsetHost(id string) error {
	return s.Mutate(s.ctx, id, func(g *models.Game) error {
		for pId, p := range g.Players {
			if p.Host {
				p.Host = false
				g.Players[pId] = p
			}
		}

		return nil
	})
}

// SetPlayerAsHost makes the given player the sole host of the game, atomically.
func (s *core) SetPlayerAsHost(id string, playerId string) error {
	return s.Mutate(s.ctx, id, func(g *models.Game) error {
		if slot, ok := g.Players[playerId]; !ok || slot.UserID == "" {
			return fmt.Errorf("slot %v of game %v is not held by a player", playerId, id)
		}

		for pId, p := range g.Players {
			p.Host = pId == playerId
			g.Players[pId] = p
		}

		return nil
	})
}
