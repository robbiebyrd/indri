package game

import (
	"fmt"
	"time"

	goaway "github.com/TwiN/go-away"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/services/events"
	sessionUtils "github.com/robbiebyrd/indri/internal/utils/session"
)

// AssignSlot claims the first empty slot in the given team for userId.
// It looks only at slots pre-declared in g.Teams[teamId].PlayerIDs so a
// player joining team A never lands in team B's slot.
func (s *Store) AssignSlot(id string, teamId string, userId string, displayName string) (string, error) {
	var assignedSlot string

	err := s.Mutate(id, func(g *models.Game) error {
		team, ok := g.Teams[teamId]
		if !ok {
			return fmt.Errorf("team %v not found in game %v", teamId, id)
		}
		for _, slotID := range team.PlayerIDs {
			player := g.Players[slotID]
			if player.UserID == "" {
				player.UserID = userId
				player.Name = goaway.Censor(displayName)
				player.Connected = true
				player.Host = !gameHasHost(g)
				g.Players[slotID] = player
				assignedSlot = slotID
				return nil
			}
		}
		return fmt.Errorf("no available player slots in team %v of game %v", teamId, id)
	})

	return assignedSlot, err
}

// RemovePlayer clears the player's slot without removing the key, preserving
// the pre-declared schema. The slot is available for reassignment after this.
func (s *Store) RemovePlayer(id string, slotId string) error {
	if err := sessionUtils.ValidateGameAndUser(id, slotId); err != nil {
		return err
	}

	return s.Mutate(id, func(g *models.Game) error {
		slot, ok := g.Players[slotId]
		if !ok {
			return fmt.Errorf("slot %v not found in game %v", slotId, id)
		}
		slot.UserID = ""
		slot.Name = ""
		slot.Connected = false
		slot.Host = false
		slot.Controller = false
		g.Players[slotId] = slot
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

// ConnectPlayer marks the player's slot as connected.
func (s *Store) ConnectPlayer(id string, slotId string) error {
	return s.markPlayerConnected(id, slotId, true)
}

// DisconnectPlayer marks the player's slot as disconnected.
func (s *Store) DisconnectPlayer(id string, slotId string) error {
	return s.markPlayerConnected(id, slotId, false)
}

// markPlayerConnected sets the player's connected status atomically, only if
// the player still exists, so a concurrent removal cannot recreate a partial
// player document. The version bump keeps it coherent with Mutate's CAS.
func (s *Store) markPlayerConnected(
	id string,
	slotId string,
	connected bool,
) error {
	if err := sessionUtils.ValidateGameAndUser(id, slotId); err != nil {
		return err
	}

	objectId, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return err
	}

	playerKey := "players." + slotId

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
		return fmt.Errorf("no player %v found in game %v", slotId, id)
	}

	s.publish(id, events.OpUpdate, [][]interface{}{{playerKey + ".connected", connected}}, nil)

	return nil
}
