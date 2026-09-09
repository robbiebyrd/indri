package game

import (
	"fmt"

	"github.com/robbiebyrd/indri/internal/models"
)

// HasHost checks to see if the game has a host already.
func (s *Store) HasHost(id string) bool {
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
func (s *Store) PlayerIsHost(id string, playerId string) bool {
	g, err := s.Get(id)
	if err != nil {
		return false
	}

	return g.Players[playerId].Host
}

// UnsetHost clears the host flag on every player, atomically.
func (s *Store) UnsetHost(id string) error {
	return s.Mutate(id, func(g *models.Game) error {
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
func (s *Store) SetPlayerAsHost(id string, playerId string) error {
	return s.Mutate(id, func(g *models.Game) error {
		if _, ok := g.Players[playerId]; !ok {
			return fmt.Errorf("player with id %v is not in game %v", playerId, id)
		}

		for pId, p := range g.Players {
			p.Host = pId == playerId
			g.Players[pId] = p
		}

		return nil
	})
}
