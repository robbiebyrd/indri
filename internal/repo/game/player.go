package game

import (
	"fmt"
	"slices"

	goaway "github.com/TwiN/go-away"

	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/services/events"
	sessionUtils "github.com/robbiebyrd/indri/internal/utils/session"
)

// HasPlayer determines if a given userId is in a game.
func (s *core) HasPlayer(id string, userId string) bool {
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
func (s *core) PlayerOnATeam(id string, userId string) bool {
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
func (s *core) AddPlayer(id string, userId string, displayName string) error {
	if err := sessionUtils.ValidateGameAndUser(id, userId); err != nil {
		return err
	}

	return s.Mutate(s.ctx, id, func(g *models.Game) error {
		if _, ok := g.Players[userId]; ok {
			return fmt.Errorf("player with id %v already exists in game %v", userId, id)
		}

		if g.Players == nil {
			g.Players = map[string]models.Player{}
		}

		g.Players[userId] = models.Player{
			Name:      goaway.Censor(displayName),
			Host:      !gameHasHost(g),
			Connected: false,
		}

		return nil
	})
}

// RemovePlayer removes a player from a game and any team it was on, atomically.
func (s *core) RemovePlayer(id string, userId string) error {
	if err := sessionUtils.ValidateGameAndUser(id, userId); err != nil {
		return err
	}

	return s.Mutate(s.ctx, id, func(g *models.Game) error {
		removePlayerFromTeams(g, userId)
		delete(g.Players, userId)

		return nil
	})
}

// gameHasHost reports whether any player in the in-memory game is the host.
func gameHasHost(g *models.Game) bool {
	for _, p := range g.Players {
		if p.Host {
			return true
		}
	}

	return false
}

// removePlayerFromTeams removes userId from every team it belongs to on the
// in-memory game, returning whether any team changed.
func removePlayerFromTeams(g *models.Game, userId string) bool {
	removed := false

	for tId, team := range g.Teams {
		newIDs := make([]string, 0, len(team.PlayerIDs))
		teamChanged := false

		for _, pId := range team.PlayerIDs {
			if pId == userId {
				teamChanged = true
				removed = true
			} else {
				newIDs = append(newIDs, pId)
			}
		}

		if teamChanged {
			team.PlayerIDs = newIDs
			g.Teams[tId] = team
		}
	}

	return removed
}

// ConnectPlayer marks the player as online.
func (s *core) ConnectPlayer(id string, userId string) error {
	return s.markPlayerConnected(id, userId, true)
}

// DisconnectPlayer marks the player as offline.
func (s *core) DisconnectPlayer(id string, userId string) error {
	return s.markPlayerConnected(id, userId, false)
}

// markPlayerConnected sets the player's connected status through the backend's
// conditional single-field write and publishes the delta. A write that never
// publishes is invisible to players, so the publish belongs here rather than
// in each backend.
func (s *core) markPlayerConnected(
	id string,
	userId string,
	connected bool,
) error {
	if err := sessionUtils.ValidateGameAndUser(id, userId); err != nil {
		return err
	}

	if err := s.docs.setPlayerConnected(s.ctx, id, userId, connected); err != nil {
		return err
	}

	s.publish(id, events.OpUpdate, map[string]interface{}{playerConnectedKey(userId): connected}, nil)

	return nil
}

// playerConnectedKey is the dotted path of one player's connected flag, shared
// by the write and the delta so the two can never disagree.
func playerConnectedKey(userId string) string {
	return "players." + userId + ".connected"
}
