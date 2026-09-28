package game

import (
	"slices"
	"testing"
)

// Slot ownership rules every store shares: a user holds at most one slot in a
// game, and only the slot's current holder can be disconnected or removed
// from it.

// slotsHeldBy returns the slots userID holds in the game.
func slotsHeldBy(t *testing.T, store Storer, id, userID string) []string {
	t.Helper()

	g, err := store.Get(id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	var slots []string
	for slot, p := range g.Players {
		if p.UserID == userID {
			slots = append(slots, slot)
		}
	}

	return slots
}

func TestStore_JoiningAgainKeepsTheSameSlot(t *testing.T) {
	eachStore(t, func(t *testing.T, store Storer) {
		g := twoSlotGame(t, store)
		first, _ := store.AssignSlot(g.ID, "red", "u1", "Alice")
		_ = store.DisconnectPlayer(g.ID, first, "u1")

		again, err := store.AssignSlot(g.ID, "red", "u1", "Alice")
		if err != nil {
			t.Fatalf("AssignSlot again: %v", err)
		}

		if again != first {
			t.Fatalf("joining again gave slot %s, want the held slot %s", again, first)
		}
		if held := slotsHeldBy(t, store, g.ID, "u1"); len(held) != 1 {
			t.Fatalf("u1 holds %v, want exactly one slot", held)
		}
		if p := player(t, store, g.ID, first); !p.Connected || !p.Host {
			t.Fatalf("slot after joining again = %+v, want connected and still host", p)
		}
	})
}

func TestStore_JoiningAnotherTeamMovesThePlayer(t *testing.T) {
	eachStore(t, func(t *testing.T, store Storer) {
		g := twoSlotGame(t, store)
		old, _ := store.AssignSlot(g.ID, "red", "u1", "Alice")

		moved, err := store.AssignSlot(g.ID, "blue", "u1", "Alice")
		if err != nil {
			t.Fatalf("AssignSlot blue: %v", err)
		}

		after, _ := store.Get(g.ID)
		if !slices.Contains(after.Teams["blue"].PlayerIDs, moved) {
			t.Fatalf("new slot %s is not one of blue's %v", moved, after.Teams["blue"].PlayerIDs)
		}
		if held := slotsHeldBy(t, store, g.ID, "u1"); len(held) != 1 || held[0] != moved {
			t.Fatalf("u1 holds %v, want only %s", held, moved)
		}
		if p := player(t, store, g.ID, old); p.UserID != "" || p.Host {
			t.Fatalf("old slot %s not freed: %+v", old, p)
		}
		if !player(t, store, g.ID, moved).Host {
			t.Fatal("the host lost the host flag by switching teams")
		}
	})
}

func TestStore_JoiningAFullTeamKeepsTheCurrentSlot(t *testing.T) {
	eachStore(t, func(t *testing.T, store Storer) {
		g := twoSlotGame(t, store)
		current, _ := store.AssignSlot(g.ID, "red", "u1", "Alice")
		_, _ = store.AssignSlot(g.ID, "blue", "u2", "Bob")
		_, _ = store.AssignSlot(g.ID, "blue", "u3", "Cy")

		if _, err := store.AssignSlot(g.ID, "blue", "u1", "Alice"); err == nil {
			t.Fatal("moving into a full team succeeded")
		}

		if held := slotsHeldBy(t, store, g.ID, "u1"); len(held) != 1 || held[0] != current {
			t.Fatalf("u1 holds %v, want still %s", held, current)
		}
	})
}

// A player who left can still have a session naming their old slot; once
// someone else takes that slot, the old session must not affect them.
func TestStore_AStaleHolderCannotDisconnectOrRemoveTheNewHolder(t *testing.T) {
	eachStore(t, func(t *testing.T, store Storer) {
		g := twoSlotGame(t, store)
		slot, _ := store.AssignSlot(g.ID, "red", "u1", "Alice")
		if err := store.RemovePlayer(g.ID, slot, "u1"); err != nil {
			t.Fatalf("RemovePlayer: %v", err)
		}
		if taken, _ := store.AssignSlot(g.ID, "red", "u2", "Bob"); taken != slot {
			t.Fatalf("u2 got %s, want the freed slot %s", taken, slot)
		}

		if err := store.DisconnectPlayer(g.ID, slot, "u1"); err != nil {
			t.Fatalf("a stale disconnect should be a no-op, got %v", err)
		}
		if !player(t, store, g.ID, slot).Connected {
			t.Fatal("the stale holder's disconnect marked the new holder disconnected")
		}

		if err := store.RemovePlayer(g.ID, slot, "u1"); err == nil {
			t.Fatal("the stale holder removed the new holder")
		}
		if p := player(t, store, g.ID, slot); p.UserID != "u2" {
			t.Fatalf("slot %s = %+v, want still held by u2", slot, p)
		}
	})
}
