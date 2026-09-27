package game

import (
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/robbiebyrd/indri/internal/models"
)

// Player behavior every game store must share. Each test runs against every
// backend available here: memory and SQLite always, MongoDB when reachable,
// PostgreSQL when INDRI_TEST_POSTGRES_URI is set.

func eachStore(t *testing.T, test func(t *testing.T, store Storer)) {
	t.Helper()

	t.Run("memory", func(t *testing.T) { test(t, newMemoryFixture(t)) })
	t.Run("sqlite", func(t *testing.T) { test(t, newSQLiteFixture(t)) })
	t.Run("mongo", func(t *testing.T) { test(t, newTestStore(t)) })
	t.Run("postgres", func(t *testing.T) { test(t, newPostgresFixture(t)) })
}

// twoSlotGame creates a game whose "red" team has two slots.
func twoSlotGame(t *testing.T, store Storer) *models.Game {
	t.Helper()

	g, err := store.New(fmt.Sprintf("contract-%d", time.Now().UnixNano()), &models.Script{
		Config: models.Config{MaxPlayersPerTeam: 2},
		Teams:  map[string]models.Team{"red": {Name: "Red"}, "blue": {Name: "Blue"}},
	}, false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	return g
}

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

func TestStore_AssignSlotFillsASlotOnTheChosenTeam(t *testing.T) {
	eachStore(t, func(t *testing.T, store Storer) {
		g := twoSlotGame(t, store)

		slot, err := store.AssignSlot(g.ID, "red", "u1", "Alice")
		if err != nil {
			t.Fatalf("AssignSlot: %v", err)
		}

		if p := player(t, store, g.ID, slot); p.UserID != "u1" || p.Name != "Alice" || !p.Connected {
			t.Fatalf("slot %s = %+v", slot, p)
		}

		after, _ := store.Get(g.ID)
		if !slices.Contains(after.Teams["red"].PlayerIDs, slot) {
			t.Fatalf("slot %s is not one of red's slots %v", slot, after.Teams["red"].PlayerIDs)
		}
	})
}

func TestStore_RemovePlayerEmptiesTheSlotButKeepsIt(t *testing.T) {
	eachStore(t, func(t *testing.T, store Storer) {
		g := twoSlotGame(t, store)
		slot, _ := store.AssignSlot(g.ID, "red", "u1", "Alice")

		if err := store.RemovePlayer(g.ID, slot, "u1"); err != nil {
			t.Fatalf("RemovePlayer: %v", err)
		}

		if p := player(t, store, g.ID, slot); p.UserID != "" || p.Connected || p.Host {
			t.Fatalf("slot %s not emptied: %+v", slot, p)
		}

		if reused, _ := store.AssignSlot(g.ID, "red", "u2", "Bob"); reused != slot {
			t.Fatalf("freed slot %s not reused; got %s", slot, reused)
		}
	})
}

func TestStore_ConnectAndDisconnectPlayer(t *testing.T) {
	eachStore(t, func(t *testing.T, store Storer) {
		g := twoSlotGame(t, store)
		slot, _ := store.AssignSlot(g.ID, "red", "u1", "Alice")

		if err := store.DisconnectPlayer(g.ID, slot, "u1"); err != nil {
			t.Fatalf("DisconnectPlayer: %v", err)
		}
		if player(t, store, g.ID, slot).Connected {
			t.Fatal("still connected")
		}

		if err := store.ConnectPlayer(g.ID, slot, "u1"); err != nil {
			t.Fatalf("ConnectPlayer: %v", err)
		}
		if !player(t, store, g.ID, slot).Connected {
			t.Fatal("not connected")
		}

		if err := store.ConnectPlayer(g.ID, "p99", "u1"); err == nil {
			t.Fatal("connecting a slot that doesn't exist succeeded")
		}
	})
}

func TestStore_FirstPlayerIsHostAndHostCanMove(t *testing.T) {
	eachStore(t, func(t *testing.T, store Storer) {
		g := twoSlotGame(t, store)
		first, _ := store.AssignSlot(g.ID, "red", "u1", "Alice")
		second, _ := store.AssignSlot(g.ID, "red", "u2", "Bob")

		if !store.PlayerIsHost(g.ID, first) || store.PlayerIsHost(g.ID, second) {
			t.Fatal("the first player to join should be the only host")
		}

		if err := store.SetPlayerAsHost(g.ID, second); err != nil {
			t.Fatalf("SetPlayerAsHost: %v", err)
		}
		if !store.PlayerIsHost(g.ID, second) {
			t.Fatal("second player is not host after SetPlayerAsHost")
		}

		if err := store.UnsetHost(g.ID); err != nil {
			t.Fatalf("UnsetHost: %v", err)
		}
		if store.HasHost(g.ID) {
			t.Fatal("game still has a host after UnsetHost")
		}
	})
}

// TestStore_ConcurrentWritesAreNotLost proves every write path (Update,
// UpdateField, and Mutate-based writers like AssignSlot) shares one
// lock-serialized, version-fenced write: all 3n concurrent writes land, none
// is rejected, and the version counts every one of them.
func TestStore_ConcurrentWritesAreNotLost(t *testing.T) {
	const n = 20

	eachStore(t, func(t *testing.T, store Storer) {
		g, err := store.New(fmt.Sprintf("contract-%d", time.Now().UnixNano()), &models.Script{
			Config: models.Config{MaxPlayersPerTeam: n},
			Teams:  map[string]models.Team{"red": {Name: "Red"}},
		}, false)
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		var wg sync.WaitGroup
		errs := make(chan error, 3*n)

		for i := 0; i < n; i++ {
			wg.Add(3)
			go func() {
				defer wg.Done()
				if err := store.UpdateField(g.ID, fmt.Sprintf("data.k%d", i), i); err != nil {
					errs <- fmt.Errorf("UpdateField(%d): %w", i, err)
				}
			}()
			go func() {
				defer wg.Done()
				if _, err := store.AssignSlot(g.ID, "red", fmt.Sprintf("user-%d", i), fmt.Sprintf("Player %d", i)); err != nil {
					errs <- fmt.Errorf("AssignSlot(%d): %w", i, err)
				}
			}()
			go func() {
				defer wg.Done()
				if err := store.Update(g.ID, &models.UpdateGame{}); err != nil {
					errs <- fmt.Errorf("Update(%d): %w", i, err)
				}
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Error(err)
		}

		got, err := store.Get(g.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}

		assigned := map[string]bool{}
		for _, p := range got.Players {
			assigned[p.UserID] = true
		}
		for i := 0; i < n; i++ {
			if v := got.PublicData[fmt.Sprintf("k%d", i)]; v != float64(i) {
				t.Errorf("data.k%d = %v, want %d", i, v, i)
			}
			if !assigned[fmt.Sprintf("user-%d", i)] {
				t.Errorf("user-%d has no slot", i)
			}
		}

		if want := int64(1 + 3*n); got.Version != want {
			t.Errorf("Version = %d, want %d (a write was lost or skipped the fence)", got.Version, want)
		}
	})
}
