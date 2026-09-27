package game

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"time"

	goaway "github.com/TwiN/go-away"

	"github.com/robbiebyrd/indri/internal/models"
	repoErrors "github.com/robbiebyrd/indri/internal/repo"
	"github.com/robbiebyrd/indri/internal/repo/ids"
	"github.com/robbiebyrd/indri/internal/services/events"
	"github.com/robbiebyrd/indri/internal/services/mutation"
	sessionUtils "github.com/robbiebyrd/indri/internal/utils/session"
)

// Game operations written once for every store. A store supplies Mutate (a
// version-fenced read-modify-write) and these build on it, so every backend
// changes games, and publishes deltas, identically.

// mutator is the part of a store the shared operations need.
type mutator interface {
	Get(id string) (*models.Game, error)
	Mutate(id string, apply func(g *models.Game) error) error
}

// newGame builds a game from the script, with every player slot pre-declared
// so positional delta paths stay stable as players join and leave. The game
// gets its own copy of the script's data: sharing its maps would let one game's
// moves leak into the script and every later game.
func newGame(code string, script *models.Script, privateGame bool) (*models.Game, error) {
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
		return g, nil
	}

	script, err := cloneScript(script)
	if err != nil {
		return nil, err
	}

	g.Players, g.Teams = preDeclareSlots(script)
	g.Stage = script.Stage

	if script.PublicData != nil {
		g.PublicData = script.PublicData
	}

	if script.PrivateData != nil {
		g.PrivateData = script.PrivateData
	}

	return g, nil
}

// cloneScript deep-copies a script through JSON, the form it was loaded from.
func cloneScript(script *models.Script) (*models.Script, error) {
	data, err := json.Marshal(script)
	if err != nil {
		return nil, fmt.Errorf("copying script: %w", err)
	}

	var clone models.Script
	if err := json.Unmarshal(data, &clone); err != nil {
		return nil, fmt.Errorf("copying script: %w", err)
	}

	return &clone, nil
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

// assignSlot seats userId in teamId and returns their slot. A user holds at
// most one slot per game: joining their own team again keeps (and reconnects)
// their slot, and joining another team moves them, host flag included, to its
// first empty slot. Only slots pre-declared in g.Teams[teamId].PlayerIDs are
// considered, so a player joining team A never lands in team B's slot.
func assignSlot(m mutator, id string, teamId string, userId string, displayName string) (string, error) {
	var assignedSlot string

	err := m.Mutate(id, func(g *models.Game) error {
		team, ok := g.Teams[teamId]
		if !ok {
			return fmt.Errorf("team %v not found in game %v", teamId, id)
		}

		held := slotHeldBy(g, userId)
		if held != "" && slices.Contains(team.PlayerIDs, held) {
			player := g.Players[held]
			player.Connected = true
			g.Players[held] = player
			assignedSlot = held
			return nil
		}

		for _, slotID := range team.PlayerIDs {
			player := g.Players[slotID]
			if player.UserID != "" {
				continue
			}

			host := !gameHasHost(g)
			if held != "" {
				host = g.Players[held].Host
				clearSlot(g, held)
			}

			player.UserID = userId
			player.Name = goaway.Censor(displayName)
			player.Connected = true
			player.Host = host
			g.Players[slotID] = player
			assignedSlot = slotID
			return nil
		}

		return fmt.Errorf("no available player slots in team %v of game %v", teamId, id)
	})

	return assignedSlot, err
}

// removePlayer empties userId's slot without removing the key, preserving the
// pre-declared schema; the slot is then free for reassignment. It fails if
// userId no longer holds the slot.
func removePlayer(m mutator, id string, slotId string, userId string) error {
	if err := sessionUtils.ValidateGameAndUser(id, slotId); err != nil {
		return err
	}

	return m.Mutate(id, func(g *models.Game) error {
		slot, ok := g.Players[slotId]
		if !ok {
			return fmt.Errorf("slot %v not found in game %v", slotId, id)
		}
		if slot.UserID != userId {
			return fmt.Errorf("slot %v of game %v is not held by %v", slotId, id, userId)
		}
		clearSlot(g, slotId)
		return nil
	})
}

// setConnected sets whether userId, in slotId, is connected. It fails rather
// than creating a missing slot, and does nothing if userId no longer holds the
// slot: a stale connection must not change its new holder.
func setConnected(m mutator, id string, slotId string, userId string, connected bool) error {
	if err := sessionUtils.ValidateGameAndUser(id, slotId); err != nil {
		return err
	}

	return m.Mutate(id, func(g *models.Game) error {
		slot, ok := g.Players[slotId]
		if !ok {
			return fmt.Errorf("no player %v found in game %v", slotId, id)
		}
		if slot.UserID != userId {
			return mutation.ErrAbort
		}
		slot.Connected = connected
		g.Players[slotId] = slot
		return nil
	})
}

// slotHeldBy returns the slot userId holds in g, or "" if none. Empty slots
// have no user, so an empty userId holds nothing.
func slotHeldBy(g *models.Game, userId string) string {
	if userId == "" {
		return ""
	}

	for slotID, p := range g.Players {
		if p.UserID == userId {
			return slotID
		}
	}

	return ""
}

// clearSlot empties a slot of its player, keeping the slot and its data.
func clearSlot(g *models.Game, slotId string) {
	slot := g.Players[slotId]
	slot.UserID = ""
	slot.Name = ""
	slot.Connected = false
	slot.Host = false
	slot.Controller = false
	g.Players[slotId] = slot
}

// update copies the set fields of upd onto the game. Private is always copied.
func update(m mutator, id string, upd *models.UpdateGame) error {
	return m.Mutate(id, func(g *models.Game) error {
		if upd.Teams != nil {
			g.Teams = *upd.Teams
		}
		if upd.Players != nil {
			g.Players = *upd.Players
		}
		if upd.Stage != nil {
			g.Stage = *upd.Stage
		}
		if upd.PublicData != nil {
			g.PublicData = upd.PublicData
		}
		if upd.PrivateData != nil {
			g.PrivateData = upd.PrivateData
		}
		if upd.PlayerData != nil {
			g.PlayerData = upd.PlayerData
		}
		g.Private = upd.Private
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

// hasHost reports whether the game has a host; an unreadable game has none.
func hasHost(m mutator, id string) bool {
	g, err := m.Get(id)
	if err != nil {
		return false
	}

	return gameHasHost(g)
}

// playerIsHost reports whether the player in slot playerId hosts the game.
func playerIsHost(m mutator, id string, playerId string) bool {
	g, err := m.Get(id)
	if err != nil {
		return false
	}

	return g.Players[playerId].Host
}

// setPlayerAsHost makes playerId the game's sole host.
func setPlayerAsHost(m mutator, id string, playerId string) error {
	return m.Mutate(id, func(g *models.Game) error {
		if _, ok := g.Players[playerId]; !ok {
			return fmt.Errorf("player %q: %w", playerId, repoErrors.ErrNotFound)
		}
		for slotId, p := range g.Players {
			p.Host = slotId == playerId
			g.Players[slotId] = p
		}
		return nil
	})
}

// unsetHost clears the host flag on every player.
func unsetHost(m mutator, id string) error {
	return m.Mutate(id, func(g *models.Game) error {
		for slotId, p := range g.Players {
			p.Host = false
			g.Players[slotId] = p
		}
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
