package game

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/robbiebyrd/indri/internal/clients/mongodb"
	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/services/events"
	"github.com/robbiebyrd/indri/internal/services/lock"
)

// newTestStore connects to a local MongoDB (a single-node replica set is not
// required for these repo-level tests). It skips the test when no database is
// reachable, so CI without Mongo stays green.
func newTestStore(t *testing.T) *Store {
	t.Helper()

	uri := os.Getenv("INDRI_TEST_MONGO_URI")
	if uri == "" {
		uri = "mongodb://localhost:27017/?directConnection=true"
	}
	_ = os.Setenv("INDRI_MONGO_URI", uri)
	_ = os.Setenv("INDRI_MONGO_DATABASE", "indri_test")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	client, err := mongodb.New(ctx)
	if err != nil {
		t.Skipf("skipping: MongoDB not reachable: %v", err)
	}

	store, err := NewStore(context.Background(), client, lock.NewInProcess(), events.NewInProcess())
	if err != nil {
		t.Skipf("skipping: could not create game store: %v", err)
	}

	return store
}

// TestAddPlayer_ConcurrentNoLostUpdates verifies that concurrent AddPlayer
// calls against the same game do not lose players — the exact lost-update race
// the optimistic-concurrency version check is meant to prevent.
func TestAddPlayer_ConcurrentNoLostUpdates(t *testing.T) {
	store := newTestStore(t)

	const players = 25

	code := fmt.Sprintf("test-%d", time.Now().UnixNano())
	script := &models.Script{
		Config: models.Config{MaxPlayersPerTeam: players},
		Teams:  map[string]models.Team{"Main": {Name: "Main"}},
	}

	g, err := store.New(code, script, false)
	if err != nil {
		t.Fatalf("creating game: %v", err)
	}

	gameId := g.ID.Hex()

	var wg sync.WaitGroup

	errs := make(chan error, players)

	for i := 0; i < players; i++ {
		wg.Add(1)

		go func(n int) {
			defer wg.Done()

			userId := fmt.Sprintf("user-%d", n)
			if _, err := store.AssignSlot(gameId, "Main", userId, userId); err != nil {
				errs <- err
			}
		}(i)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent AssignSlot failed: %v", err)
	}

	final, err := store.Get(gameId)
	if err != nil {
		t.Fatalf("reloading game: %v", err)
	}

	assigned := 0
	for _, p := range final.Players {
		if p.UserID != "" {
			assigned++
		}
	}
	if assigned != players {
		t.Fatalf("expected %d assigned players after concurrent assigns, got %d (lost updates)", players, assigned)
	}

	hosts := 0
	for _, p := range final.Players {
		if p.Host {
			hosts++
		}
	}

	if hosts != 1 {
		t.Errorf("expected exactly one host, got %d", hosts)
	}
}
