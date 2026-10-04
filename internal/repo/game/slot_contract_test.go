package game

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/robbiebyrd/indri/internal/models"
)

// Slot rules every store shares. A game's player slots are declared when it is
// created, so membership is a slot changing hands rather than a key appearing
// or vanishing: a user holds at most one slot in a game, a slot belongs to
// exactly one team, and only a slot's current holder can be disconnected from
// it or removed from it.

// twoSlotScript gives "red" and "blue" two slots each. Teams are assigned
// slots in name order, so blue holds p0 and p1 and red holds p2 and p3.
func twoSlotScript() *models.Script {
	return &models.Script{
		Config: models.Config{MaxPlayersPerTeam: 2},
		Teams:  map[string]models.Team{"red": {Name: "Red"}, "blue": {Name: "Blue"}},
	}
}

func twoSlotGame(t *testing.T, store Storer) *models.Game {
	t.Helper()

	return newGame(t, store, twoSlotScript())
}

// player returns the player in slot, failing if the slot does not exist.
func player(t *testing.T, store Storer, id, slot string) models.Player {
	t.Helper()

	g, err := store.Get(id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	p, ok := g.Players[slot]
	if !ok {
		t.Fatalf("slot %q missing from game", slot)
	}

	return p
}

// slotsHeldBy returns the slots userId holds in the game.
func slotsHeldBy(t *testing.T, store Storer, id, userId string) []string {
	t.Helper()

	g, err := store.Get(id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	var slots []string

	for slot, p := range g.Players {
		if p.UserID == userId {
			slots = append(slots, slot)
		}
	}

	return slots
}

// assign seats userId on teamId and fails the test if it cannot.
func assign(t *testing.T, store Storer, id, teamId, userId string) string {
	t.Helper()

	slot, _, err := store.AssignSlot(id, teamId, userId, userId)
	if err != nil {
		t.Fatalf("AssignSlot(%s, %s): %v", teamId, userId, err)
	}

	return slot
}

func TestNew_PreDeclaresEveryTeamsSlots(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b backend) {
		script := twoSlotScript()
		g := newGame(t, b.store, script)

		if got := g.Teams["blue"].PlayerIDs; !slices.Equal(got, []string{"p0", "p1"}) {
			t.Errorf("blue slots = %v, want [p0 p1]", got)
		}

		if got := g.Teams["red"].PlayerIDs; !slices.Equal(got, []string{"p2", "p3"}) {
			t.Errorf("red slots = %v, want [p2 p3]", got)
		}

		if len(g.Players) != 4 {
			t.Fatalf("game has %d slots, want 4: %v", len(g.Players), g.Players)
		}

		for slot, p := range g.Players {
			if p.UserID != "" || p.Host || p.Connected {
				t.Errorf("new slot %s is not empty: %+v", slot, p)
			}
		}

		// Declaring slots must not write them into the script every later game
		// is stamped from.
		for name, team := range script.Teams {
			if len(team.PlayerIDs) != 0 {
				t.Errorf("creating a game wrote slots %v into the script's team %s", team.PlayerIDs, name)
			}
		}
	})
}

func TestAssignSlot_SeatsTheUserOnTheChosenTeam(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b backend) {
		g := twoSlotGame(t, b.store)

		slot, seated, err := b.store.AssignSlot(g.ID, "red", "u1", "Alice")
		if err != nil {
			t.Fatalf("AssignSlot: %v", err)
		}

		if !seated {
			t.Error("a user's first slot in the game was not reported as seating them")
		}

		if !slices.Contains(g.Teams["red"].PlayerIDs, slot) {
			t.Fatalf("slot %s is not one of red's %v", slot, g.Teams["red"].PlayerIDs)
		}

		if p := player(t, b.store, g.ID, slot); p.UserID != "u1" || p.Name != "Alice" || !p.Connected || !p.Host {
			t.Fatalf("slot %s = %+v, want u1/Alice, connected, and host as the first player", slot, p)
		}
	})
}

func TestAssignSlot_RefusesATeamTheGameDoesNotHave(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b backend) {
		g := twoSlotGame(t, b.store)

		if _, _, err := b.store.AssignSlot(g.ID, "green", "u1", "Alice"); err == nil {
			t.Fatal("seating a user on a team the game does not have succeeded")
		}

		if held := slotsHeldBy(t, b.store, g.ID, "u1"); len(held) != 0 {
			t.Fatalf("u1 holds %v after a refused join", held)
		}
	})
}

func TestAssignSlot_JoiningAgainKeepsTheSameSlot(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b backend) {
		g := twoSlotGame(t, b.store)
		first := assign(t, b.store, g.ID, "red", "u1")

		if err := b.store.DisconnectPlayer(g.ID, first, "u1"); err != nil {
			t.Fatalf("DisconnectPlayer: %v", err)
		}

		again, seated, err := b.store.AssignSlot(g.ID, "red", "u1", "u1")
		if err != nil {
			t.Fatalf("AssignSlot again: %v", err)
		}

		if seated {
			t.Error("rejoining a held slot was reported as seating the user again")
		}

		if again != first {
			t.Fatalf("joining again gave slot %s, want the held slot %s", again, first)
		}

		if held := slotsHeldBy(t, b.store, g.ID, "u1"); len(held) != 1 {
			t.Fatalf("u1 holds %v, want exactly one slot", held)
		}

		if p := player(t, b.store, g.ID, first); !p.Connected || !p.Host {
			t.Fatalf("slot after joining again = %+v, want connected and still host", p)
		}
	})
}

func TestAssignSlot_JoiningAnotherTeamMovesThePlayer(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b backend) {
		g := twoSlotGame(t, b.store)
		old := assign(t, b.store, g.ID, "red", "u1")

		moved, seated, err := b.store.AssignSlot(g.ID, "blue", "u1", "u1")
		if err != nil {
			t.Fatalf("AssignSlot blue: %v", err)
		}

		if seated {
			t.Error("changing team was reported as seating the user again")
		}

		if !slices.Contains(g.Teams["blue"].PlayerIDs, moved) {
			t.Fatalf("new slot %s is not one of blue's %v", moved, g.Teams["blue"].PlayerIDs)
		}

		if held := slotsHeldBy(t, b.store, g.ID, "u1"); len(held) != 1 || held[0] != moved {
			t.Fatalf("u1 holds %v, want only %s", held, moved)
		}

		if p := player(t, b.store, g.ID, old); p.UserID != "" || p.Host || p.Connected {
			t.Fatalf("old slot %s not freed: %+v", old, p)
		}

		if !player(t, b.store, g.ID, moved).Host {
			t.Fatal("the host lost the host flag by switching teams")
		}
	})
}

func TestAssignSlot_JoiningAFullTeamKeepsTheCurrentSlot(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b backend) {
		g := twoSlotGame(t, b.store)
		current := assign(t, b.store, g.ID, "red", "u1")
		assign(t, b.store, g.ID, "blue", "u2")
		assign(t, b.store, g.ID, "blue", "u3")

		if _, _, err := b.store.AssignSlot(g.ID, "blue", "u1", "u1"); err == nil {
			t.Fatal("moving into a full team succeeded")
		}

		if held := slotsHeldBy(t, b.store, g.ID, "u1"); len(held) != 1 || held[0] != current {
			t.Fatalf("u1 holds %v, want still %s", held, current)
		}
	})
}

func TestRemovePlayer_EmptiesTheSlotButKeepsIt(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b backend) {
		g := twoSlotGame(t, b.store)
		slot := assign(t, b.store, g.ID, "red", "u1")

		if err := b.store.RemovePlayer(g.ID, slot, "u1"); err != nil {
			t.Fatalf("RemovePlayer: %v", err)
		}

		if p := player(t, b.store, g.ID, slot); p.UserID != "" || p.Name != "" || p.Connected || p.Host {
			t.Fatalf("slot %s not emptied: %+v", slot, p)
		}

		if reused := assign(t, b.store, g.ID, "red", "u2"); reused != slot {
			t.Fatalf("freed slot %s not reused; got %s", slot, reused)
		}
	})
}

// TestRemovePlayer_SucceedsOnceUnderARace: a leave and a kick racing must
// between them remove the player once, because player:left is raised on a
// removal that succeeded.
func TestRemovePlayer_SucceedsOnceUnderARace(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b backend) {
		g := twoSlotGame(t, b.store)
		slot := assign(t, b.store, g.ID, "red", "u1")

		const racers = 8

		var (
			wg        sync.WaitGroup
			mu        sync.Mutex
			succeeded int
		)

		for i := 0; i < racers; i++ {
			wg.Add(1)

			go func() {
				defer wg.Done()

				if err := b.store.RemovePlayer(g.ID, slot, "u1"); err == nil {
					mu.Lock()
					succeeded++
					mu.Unlock()
				}
			}()
		}

		wg.Wait()

		if succeeded != 1 {
			t.Fatalf("%d of %d racing removals succeeded, want exactly 1", succeeded, racers)
		}
	})
}

func TestConnectAndDisconnectPlayer(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b backend) {
		g := twoSlotGame(t, b.store)
		slot := assign(t, b.store, g.ID, "red", "u1")

		b.events.drain()

		if err := b.store.DisconnectPlayer(g.ID, slot, "u1"); err != nil {
			t.Fatalf("DisconnectPlayer: %v", err)
		}

		if player(t, b.store, g.ID, slot).Connected {
			t.Fatal("still connected")
		}

		if err := b.store.ConnectPlayer(g.ID, slot, "u1"); err != nil {
			t.Fatalf("ConnectPlayer: %v", err)
		}

		if !player(t, b.store, g.ID, slot).Connected {
			t.Fatal("not connected")
		}

		if updated := b.events.updated(); updated["players."+slot+".connected"] != true {
			t.Errorf("connecting published %v, want players.%s.connected true", updated, slot)
		}

		if err := b.store.ConnectPlayer(g.ID, "p99", "u1"); err == nil {
			t.Fatal("connecting a slot that doesn't exist succeeded")
		}
	})
}

// A player who left can still have a session naming their old slot; once
// someone else takes that slot, the old session must not affect them.
func TestSlot_AStaleHolderCannotDisconnectOrRemoveTheNewHolder(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b backend) {
		g := twoSlotGame(t, b.store)
		slot := assign(t, b.store, g.ID, "red", "u1")

		if err := b.store.RemovePlayer(g.ID, slot, "u1"); err != nil {
			t.Fatalf("RemovePlayer: %v", err)
		}

		if taken := assign(t, b.store, g.ID, "red", "u2"); taken != slot {
			t.Fatalf("u2 got %s, want the freed slot %s", taken, slot)
		}

		b.events.drain()

		if err := b.store.DisconnectPlayer(g.ID, slot, "u1"); err != nil {
			t.Fatalf("a stale disconnect should be a no-op, got %v", err)
		}

		if !player(t, b.store, g.ID, slot).Connected {
			t.Fatal("the stale holder's disconnect marked the new holder disconnected")
		}

		if published := b.events.drain(); len(published) != 0 {
			t.Errorf("a stale disconnect published %v", published)
		}

		if err := b.store.RemovePlayer(g.ID, slot, "u1"); !errors.Is(err, ErrSlotNotHeld) {
			t.Fatalf("the stale holder's removal = %v, want ErrSlotNotHeld", err)
		}

		if p := player(t, b.store, g.ID, slot); p.UserID != "u2" {
			t.Fatalf("slot %s = %+v, want still held by u2", slot, p)
		}
	})
}

// markSlot gives slot a score and both kinds of player data, as a game in
// progress would.
func markSlot(t *testing.T, store Storer, id, slot string) {
	t.Helper()

	if err := store.Mutate(context.Background(), id, func(g *models.Game) error {
		p := g.Players[slot]
		p.Score = 7
		p.PublicData = &map[string]interface{}{"avatar": "fox"}
		p.PrivateData = &map[string]interface{}{"hand": []interface{}{"ace"}}
		g.Players[slot] = p

		return nil
	}); err != nil {
		t.Fatalf("marking slot %s: %v", slot, err)
	}
}

// A slot outlives its holder, but what the holder had does not pass to the
// next user seated there: a card game's departed player must not hand their
// hand and score to whoever joins next.
func TestAssignSlot_ANewHolderStartsClean(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b backend) {
		g := twoSlotGame(t, b.store)
		slot := assign(t, b.store, g.ID, "red", "alice")
		markSlot(t, b.store, g.ID, slot)

		if err := b.store.RemovePlayer(g.ID, slot, "alice"); err != nil {
			t.Fatalf("RemovePlayer: %v", err)
		}

		// Between the departure and the next join, the slot still says what
		// alice had, so a player:left subscriber can read it.
		if p := player(t, b.store, g.ID, slot); p.Score != 7 || p.PrivateData == nil {
			t.Fatalf("a freed slot lost its holder's data before anyone took it: %+v", p)
		}

		if taken := assign(t, b.store, g.ID, "red", "bob"); taken != slot {
			t.Fatalf("bob got %s, want the freed slot %s", taken, slot)
		}

		if p := player(t, b.store, g.ID, slot); p.Score != 0 || p.PublicData != nil || p.PrivateData != nil {
			t.Fatalf("bob inherited alice's slot state: %+v", p)
		}
	})
}

// Rejoining the slot you already hold is a reconnect, not a new player.
func TestAssignSlot_RejoiningKeepsTheHoldersData(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b backend) {
		g := twoSlotGame(t, b.store)
		slot := assign(t, b.store, g.ID, "red", "alice")
		markSlot(t, b.store, g.ID, slot)

		assign(t, b.store, g.ID, "red", "alice")

		if p := player(t, b.store, g.ID, slot); p.Score != 7 || p.PrivateData == nil || p.PublicData == nil {
			t.Fatalf("rejoining wiped the holder's own state: %+v", p)
		}
	})
}

// RemovePlayerIf runs its guard inside the write, against the game as it is
// about to be committed — not a copy read earlier — so a condition that stopped
// holding in the meantime refuses the removal.
func TestRemovePlayerIf_GuardsAgainstTheCommittedGame(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b backend) {
		g := twoSlotGame(t, b.store)
		host := assign(t, b.store, g.ID, "red", "host")
		target := assign(t, b.store, g.ID, "red", "target")

		hostStillHosts := func(g *models.Game) error {
			if !g.Players[host].Host {
				return errors.New("the caller is no longer the host")
			}

			return nil
		}

		// The host hands the role to the target before the removal runs.
		if err := b.store.SetPlayerAsHost(g.ID, target); err != nil {
			t.Fatalf("SetPlayerAsHost: %v", err)
		}

		if err := b.store.RemovePlayerIf(g.ID, target, "target", hostStillHosts); err == nil {
			t.Fatal("the removal ran although its guard no longer held")
		}

		if p := player(t, b.store, g.ID, target); p.UserID != "target" {
			t.Fatalf("a refused removal still freed the slot: %+v", p)
		}

		if err := b.store.SetPlayerAsHost(g.ID, host); err != nil {
			t.Fatalf("SetPlayerAsHost back: %v", err)
		}

		if err := b.store.RemovePlayerIf(g.ID, target, "target", hostStillHosts); err != nil {
			t.Fatalf("RemovePlayerIf with its guard holding: %v", err)
		}

		if p := player(t, b.store, g.ID, target); p.UserID != "" {
			t.Fatalf("slot %s not freed: %+v", target, p)
		}
	})
}
