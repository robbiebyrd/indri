package game

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"

	goaway "github.com/TwiN/go-away"

	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/services/mutation"
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

// ErrSlotNotHeld is a removal refused because the user no longer holds the
// slot. To a caller freeing their own seat it means the seat is already gone —
// a kick or another connection got there first — which is not a failure.
var ErrSlotNotHeld = errors.New("the slot is not held by that user")

// AssignSlot seats userId on teamId and returns their slot.
//
// A user holds at most one slot per game. Joining the team they are already on
// keeps (and reconnects) their slot; joining another team moves them, host flag
// included, to its first free slot. Only the team's own pre-declared slots are
// considered, so a player joining red never lands in blue's slot. A full team
// is an error and leaves the user where they were.
//
// seated reports whether this call is what put the user in the game — they held
// no slot before it and the write committed. GameService raises player:joined
// from it, and a rejoin or a team change must not raise it again: a subscriber
// that dealt them a hand would deal a second one. It is assigned on every run
// of apply because mutation re-runs apply against the game that won each
// version fence, and the answer belongs to the attempt that committed.
func (s *core) AssignSlot(id string, teamId string, userId string, displayName string) (string, bool, error) {
	if err := sessionUtils.ValidateGameAndUser(id, userId); err != nil {
		return "", false, err
	}

	var (
		assigned string
		newcomer bool
	)

	committed, err := s.MutateResult(s.ctx, id, func(g *models.Game) error {
		team, ok := g.Teams[teamId]
		if !ok {
			return fmt.Errorf("team %v not found in game %v", teamId, id)
		}

		held := slotHeldBy(g, userId)
		newcomer = held == ""

		if held != "" && slices.Contains(team.PlayerIDs, held) {
			player := g.Players[held]
			player.Connected = true
			g.Players[held] = player
			assigned = held

			return nil
		}

		for _, slotId := range team.PlayerIDs {
			if g.Players[slotId].UserID != "" {
				continue
			}

			host := !gameHasHost(g)
			if held != "" {
				host = g.Players[held].Host
				clearSlot(g, held)
			}

			// A fresh player, not the old one edited: the slot's previous
			// holder's score and data were theirs, and stay readable only until
			// somebody else takes the slot.
			g.Players[slotId] = models.Player{
				UserID:    userId,
				Name:      censor.Censor(displayName),
				Connected: true,
				Host:      host,
			}
			assigned = slotId

			return nil
		}

		return fmt.Errorf("no available player slots in team %v of game %v", teamId, id)
	})

	return assigned, newcomer && committed, err
}

// RemovePlayer frees userId's slot. The slot itself stays — the game's slots
// are fixed when it is created — and is then open to the next player who joins
// that team.
//
// It fails if userId does not hold the slot, which is what makes a removal
// single-shot: of a leave and a kick racing each other exactly one succeeds,
// and GameService raises player:left on that one. It also stops a stale session
// that still names the slot from removing whoever holds it now.
func (s *core) RemovePlayer(id string, slotId string, userId string) error {
	return s.RemovePlayerIf(id, slotId, userId, nil)
}

// RemovePlayerIf is RemovePlayer, refused unless allowed accepts the game. The
// check runs inside the write, against the game about to be committed and on
// every retry, so a condition checked earlier on a copy — that the caller is
// the host, say — cannot have stopped holding by the time the slot is freed.
// A nil allowed accepts every game.
func (s *core) RemovePlayerIf(id string, slotId string, userId string, allowed func(*models.Game) error) error {
	if err := requireGameAndSlot(id, slotId); err != nil {
		return err
	}

	return s.Mutate(s.ctx, id, func(g *models.Game) error {
		if allowed != nil {
			if err := allowed(g); err != nil {
				return err
			}
		}

		slot, ok := g.Players[slotId]
		if !ok {
			return fmt.Errorf("slot %v not found in game %v", slotId, id)
		}

		if slot.UserID != userId {
			return fmt.Errorf("slot %v of game %v is not held by %v: %w", slotId, id, userId, ErrSlotNotHeld)
		}

		clearSlot(g, slotId)

		return nil
	})
}

// ConnectPlayer marks userId, in slotId, as online.
func (s *core) ConnectPlayer(id string, slotId string, userId string) error {
	return s.setConnected(id, slotId, userId, true)
}

// DisconnectPlayer marks userId, in slotId, as offline.
func (s *core) DisconnectPlayer(id string, slotId string, userId string) error {
	return s.setConnected(id, slotId, userId, false)
}

// setConnected sets whether userId, in slotId, is connected. It fails rather
// than creating a missing slot, and writes nothing if userId no longer holds
// the slot: a stale connection must not change its new holder, and a reconnect
// racing a kick must not reseat the kicked player.
func (s *core) setConnected(id string, slotId string, userId string, connected bool) error {
	if err := requireGameAndSlot(id, slotId); err != nil {
		return err
	}

	return s.Mutate(s.ctx, id, func(g *models.Game) error {
		slot, ok := g.Players[slotId]
		if !ok {
			return fmt.Errorf("slot %v not found in game %v", slotId, id)
		}

		if slot.UserID != userId {
			return mutation.ErrAbort
		}

		slot.Connected = connected
		g.Players[slotId] = slot

		return nil
	})
}

// requireGameAndSlot rejects an empty game or slot id, naming the one that is
// missing.
func requireGameAndSlot(id string, slotId string) error {
	if id == "" {
		return fmt.Errorf("game id is required")
	}

	if slotId == "" {
		return fmt.Errorf("slot id is required")
	}

	return nil
}

// declareSlots gives every team its fixed run of player slots. Teams are taken
// in name order and team i gets slots p{i*perTeam} … p{(i+1)*perTeam-1}, so
// slot ids — and the delta paths built from them — never move as players join
// and leave. teams is a fresh map: the script's own must not be written, or
// every later game stamped from it would inherit these slots.
func declareSlots(
	scriptTeams map[string]models.Team,
	perTeam int,
) (teams map[string]models.Team, players map[string]models.Player) {
	teams = make(map[string]models.Team, len(scriptTeams))
	players = make(map[string]models.Player, len(scriptTeams)*perTeam)

	names := make([]string, 0, len(scriptTeams))
	for name := range scriptTeams {
		names = append(names, name)
	}

	sort.Strings(names)

	next := 0

	for _, name := range names {
		team := scriptTeams[name]
		team.PlayerIDs = make([]string, 0, perTeam)

		for range perTeam {
			slotId := "p" + strconv.Itoa(next)
			players[slotId] = models.Player{}
			team.PlayerIDs = append(team.PlayerIDs, slotId)
			next++
		}

		teams[name] = team
	}

	return teams, players
}

// slotHeldBy returns the slot userId holds in g, or "" if none. Free slots have
// no user, so an empty userId holds nothing.
func slotHeldBy(g *models.Game, userId string) string {
	if userId == "" {
		return ""
	}

	for slotId, p := range g.Players {
		if p.UserID == userId {
			return slotId
		}
	}

	return ""
}

// clearSlot frees a slot of its player, keeping the slot and its data.
func clearSlot(g *models.Game, slotId string) {
	slot := g.Players[slotId]
	slot.UserID = ""
	slot.Name = ""
	slot.Connected = false
	slot.Host = false
	slot.Controller = false
	g.Players[slotId] = slot
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
