package game

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	goaway "github.com/TwiN/go-away"

	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/repo/ids"
	"github.com/robbiebyrd/indri/internal/services/events"
	sessionUtils "github.com/robbiebyrd/indri/internal/utils/session"
)

// Game operations written once for every store. A store supplies Mutate (a
// version-fenced read-modify-write) and these build on it, so every backend
// changes games, and publishes deltas, identically.

// mutator is the part of a store the shared operations need.
type mutator interface {
	Mutate(id string, apply func(g *models.Game) error) error
}

// newGame builds a game from the script, with every player slot pre-declared
// so positional delta paths stay stable as players join and leave.
func newGame(code string, script *models.Script, privateGame bool) *models.Game {
	now := time.Now()

	g := &models.Game{
		ID:          ids.New(),
		Version:     1,
		Code:        code,
		Teams:       map[string]models.Team{},
		Players:     map[string]models.Player{},
		PublicData:  map[string]interface{}{},
		PrivateData: map[string]interface{}{},
		PlayerData:  map[string]interface{}{},
		Private:     privateGame,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if script == nil {
		return g
	}

	g.Players, g.Teams = preDeclareSlots(script)
	g.Stage = script.Stage

	if script.PublicData != nil {
		g.PublicData = script.PublicData
	}

	if script.PrivateData != nil {
		g.PrivateData = script.PrivateData
	}

	return g
}

// preDeclareSlots creates the fixed player-slot map from the script config.
// Teams are sorted alphanumerically; team i gets slots [i*max … (i+1)*max-1].
// Returns the players map and a copy of the teams map with PlayerIDs pre-populated.
func preDeclareSlots(script *models.Script) (map[string]models.Player, map[string]models.Team) {
	players := make(map[string]models.Player)
	teams := make(map[string]models.Team, len(script.Teams))

	for k, v := range script.Teams {
		teams[k] = v
	}

	sortedNames := make([]string, 0, len(script.Teams))
	for name := range script.Teams {
		sortedNames = append(sortedNames, name)
	}
	sort.Strings(sortedNames)

	idx := 0
	for _, name := range sortedNames {
		team := teams[name]
		team.PlayerIDs = make([]string, 0, script.Config.MaxPlayersPerTeam)
		for j := 0; j < script.Config.MaxPlayersPerTeam; j++ {
			slotID := "p" + strconv.Itoa(idx)
			players[slotID] = models.Player{}
			team.PlayerIDs = append(team.PlayerIDs, slotID)
			idx++
		}
		teams[name] = team
	}

	return players, teams
}

// assignSlot claims the first empty slot in the given team for userId.
// It looks only at slots pre-declared in g.Teams[teamId].PlayerIDs so a
// player joining team A never lands in team B's slot.
func assignSlot(m mutator, id string, teamId string, userId string, displayName string) (string, error) {
	var assignedSlot string

	err := m.Mutate(id, func(g *models.Game) error {
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

// removePlayer clears the player's slot without removing the key, preserving
// the pre-declared schema. The slot is available for reassignment after this.
func removePlayer(m mutator, id string, slotId string) error {
	if err := sessionUtils.ValidateGameAndUser(id, slotId); err != nil {
		return err
	}

	return m.Mutate(id, func(g *models.Game) error {
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

// setConnected sets a slot's connected status, failing rather than creating
// the slot if it doesn't exist.
func setConnected(m mutator, id string, slotId string, connected bool) error {
	if err := sessionUtils.ValidateGameAndUser(id, slotId); err != nil {
		return err
	}

	return m.Mutate(id, func(g *models.Game) error {
		slot, ok := g.Players[slotId]
		if !ok {
			return fmt.Errorf("no player %v found in game %v", slotId, id)
		}
		slot.Connected = connected
		g.Players[slotId] = slot
		return nil
	})
}

// updateField sets the value at a dotted JSON path (e.g. "data.board.1").
func updateField(m mutator, id string, key string, value interface{}) error {
	return m.Mutate(id, func(g *models.Game) error {
		return editPath(g, key, value, false)
	})
}

// deleteField removes the value at a dotted JSON path.
func deleteField(m mutator, id string, key string) error {
	return m.Mutate(id, func(g *models.Game) error {
		return editPath(g, key, nil, true)
	})
}

// editPath applies a dotted-path set or delete to g through its JSON form,
// so paths use the same names clients see. Fields hidden from JSON would be
// zeroed by the round trip, so they are carried over.
func editPath(g *models.Game, key string, value interface{}, del bool) error {
	doc, err := events.ToMap(g)
	if err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}

	applyDottedPath(doc, key, value, del)

	version, deletedAt := g.Version, g.DeletedAt

	if err := fromMap(doc, g); err != nil {
		return fmt.Errorf("rehydrate: %w", err)
	}

	g.Version, g.DeletedAt = version, deletedAt

	return nil
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
