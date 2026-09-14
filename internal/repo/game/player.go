package game

import (
	"fmt"
	"slices"

	goaway "github.com/TwiN/go-away"

	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/services/events"
	sessionUtils "github.com/robbiebyrd/indri/internal/utils/session"
)

// censor is the display-name filter, built once at package initialisation.
//
// goaway's package-level Censor builds its detector lazily on first use and
// does so without a lock, so two players joining different games at the same
// moment race on that write — a real race in a server whose whole purpose is
// concurrent joins, and one the -race detector reports the first time two
// joins overlap. A detector built here is written once before anything runs and
// only read afterwards: Censor takes no lock because it needs none, reading the
// word lists and mutating nothing.
var censor = goaway.NewProfanityDetector()

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
	_, err := s.AddPlayerResult(id, userId, displayName)

	return err
}

// AddPlayerResult is AddPlayer, reporting whether the player was actually
// added.
//
// It exists for the same reason MutateResult does, and the caller is the same
// kind of caller: something with a side effect that may only happen if the
// write did. GameService.ConnectPlayer raises player:joined from here, and a
// player rejoining a game they are already in must not raise it a second time
// — a subscriber that dealt them a hand would deal a second one on every
// reconnect. A nil error does not answer that question on its own, which is
// precisely why the flag is threaded out rather than inferred.
func (s *core) AddPlayerResult(id string, userId string, displayName string) (bool, error) {
	if err := sessionUtils.ValidateGameAndUser(id, userId); err != nil {
		return false, err
	}

	return s.MutateResult(s.ctx, id, func(g *models.Game) error {
		if _, ok := g.Players[userId]; ok {
			return fmt.Errorf("player with id %v already exists in game %v", userId, id)
		}

		if g.Players == nil {
			g.Players = map[string]models.Player{}
		}

		g.Players[userId] = models.Player{
			Name:      censor.Censor(displayName),
			Host:      !gameHasHost(g),
			Connected: false,
		}

		return nil
	})
}

// RemovePlayer removes a player from a game and any team it was on, atomically.
func (s *core) RemovePlayer(id string, userId string) error {
	_, err := s.RemovePlayerResult(id, userId)

	return err
}

// RemovePlayerResult is RemovePlayer, reporting whether this call is why the
// player is no longer in the game.
//
// Two conditions, and both are needed. The player has to have been there when
// the winning attempt ran — removing somebody who already left writes an
// unchanged document and would otherwise look identical to a real removal — and
// that attempt has to have committed. GameService.RemovePlayer raises
// player:left from this, and a leave, a kick and a second leave racing each
// other must between them raise it exactly once.
//
// present is assigned on every run of apply rather than only the first, because
// mutation re-runs apply against the game that actually won each version fence:
// the answer belongs to the attempt that committed, not to the one that lost.
func (s *core) RemovePlayerResult(id string, userId string) (bool, error) {
	if err := sessionUtils.ValidateGameAndUser(id, userId); err != nil {
		return false, err
	}

	var present bool

	committed, err := s.MutateResult(s.ctx, id, func(g *models.Game) error {
		_, present = g.Players[userId]

		removePlayerFromTeams(g, userId)
		delete(g.Players, userId)

		return nil
	})

	return present && committed, err
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
