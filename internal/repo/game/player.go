package game

import (
	"fmt"
	"slices"
	"time"

	goaway "github.com/TwiN/go-away"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/robbiebyrd/indri/internal/models"
	sessionUtils "github.com/robbiebyrd/indri/internal/utils/session"
)

// HasPlayer determines if a given userId is in a game.
func (s *Store) HasPlayer(id string, userId string) bool {
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
func (s *Store) PlayerOnATeam(id string, userId string) bool {
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
func (s *Store) AddPlayer(id string, userId string, displayName string) error {
	if err := sessionUtils.ValidateGameAndUser(id, userId); err != nil {
		return err
	}

	return s.Mutate(id, func(g *models.Game) error {
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
func (s *Store) RemovePlayer(id string, userId string) error {
	if err := sessionUtils.ValidateGameAndUser(id, userId); err != nil {
		return err
	}

	return s.Mutate(id, func(g *models.Game) error {
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

// ConnectPlayer marks the player as offline.
func (s *Store) ConnectPlayer(id string, userId string) error {
	return s.markPlayerConnected(id, userId, true)
}

// DisconnectPlayer marks the player as offline.
func (s *Store) DisconnectPlayer(id string, userId string) error {
	return s.markPlayerConnected(id, userId, false)
}

// markPlayerConnected sets the player's connected status atomically, only if
// the player still exists, so a concurrent removal cannot recreate a partial
// player document. The version bump keeps it coherent with Mutate's CAS.
func (s *Store) markPlayerConnected(
	id string,
	userId string,
	connected bool,
) error {
	if err := sessionUtils.ValidateGameAndUser(id, userId); err != nil {
		return err
	}

	objectId, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return err
	}

	playerKey := "players." + userId

	result, err := s.collection.Collection().UpdateOne(
		*s.ctx,
		bson.D{
			{Key: "_id", Value: objectId},
			{Key: playerKey, Value: bson.D{{Key: "$exists", Value: true}}},
		},
		bson.D{
			{Key: "$set", Value: bson.D{
				{Key: playerKey + ".connected", Value: connected},
				{Key: "updatedAt", Value: time.Now()},
			}},
			{Key: "$inc", Value: bson.D{{Key: "version", Value: 1}}},
		},
	)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("no player %v found in game %v", userId, id)
	}

	return nil
}
