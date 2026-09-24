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

// capturePublisher collects ChangeEvents for test assertions.
type capturePublisher struct {
	mu     sync.Mutex
	events []events.ChangeEvent
}

func (p *capturePublisher) Publish(_ context.Context, e events.ChangeEvent) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, e)
	return nil
}

func (p *capturePublisher) Subscribe(_ context.Context) (<-chan events.ChangeEvent, error) {
	return make(chan events.ChangeEvent), nil
}

func (p *capturePublisher) last() *events.ChangeEvent {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.events) == 0 {
		return nil
	}
	e := p.events[len(p.events)-1]
	return &e
}

func (p *capturePublisher) reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = nil
}

func newTestStoreWith(t *testing.T, publisher events.Publisher) *Store {
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

	store, err := NewStore(context.Background(), client, lock.NewInProcess(), publisher)
	if err != nil {
		t.Skipf("skipping: could not create game store: %v", err)
	}

	return store
}

func makeTestGame(t *testing.T, store *Store, prefix string) *models.Game {
	t.Helper()
	code := fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	script := &models.Script{
		Config: models.Config{MaxPlayersPerTeam: 1},
		Teams:  map[string]models.Team{"A": {Name: "A"}},
	}
	g, err := store.New(code, script, false)
	if err != nil {
		t.Fatalf("creating game: %v", err)
	}
	return g
}

// assertPositionalPath verifies that path is a []interface{} (positional encoding),
// not a plain string.
func assertPositionalPath(t *testing.T, label string, path interface{}) {
	t.Helper()
	if s, ok := path.(string); ok {
		t.Errorf("%s: expected positional []interface{} path, got string %q", label, s)
		return
	}
	if _, ok := path.([]interface{}); !ok {
		t.Errorf("%s: expected []interface{} path, got %T: %v", label, path, path)
	}
}

// TestPublishDiff_RemovedPathEncoded verifies that publishDiff encodes removed-key
// paths using the before-state schema, so the removed key's position is known even
// though the key is absent from the after-state.
func TestPublishDiff_RemovedPathEncoded(t *testing.T) {
	pub := &capturePublisher{}
	store := newTestStoreWith(t, pub)

	g := makeTestGame(t, store, "removed-path")
	gameId := g.ID.Hex()

	// Add a field via Mutate.
	if err := store.Mutate(gameId, func(game *models.Game) error {
		if game.PublicData == nil {
			game.PublicData = map[string]interface{}{"tempKey": "value"}
		} else {
			game.PublicData["tempKey"] = "value"
		}
		return nil
	}); err != nil {
		t.Fatalf("initial mutate: %v", err)
	}

	pub.reset()

	// Remove that field via Mutate — publishDiff must encode the removed path
	// using the before-state schema (where tempKey existed).
	if err := store.Mutate(gameId, func(game *models.Game) error {
		delete(game.PublicData, "tempKey")
		return nil
	}); err != nil {
		t.Fatalf("delete mutate: %v", err)
	}

	ev := pub.last()
	if ev == nil {
		t.Fatal("no event published after removal mutation")
	}

	for _, path := range ev.RemovedFields {
		encoded, ok := path.([]interface{})
		if !ok {
			t.Errorf("removed path should be positional []interface{}, got %T: %v", path, path)
			continue
		}
		// The last segment must be an int (positional index), not the raw key name.
		last := encoded[len(encoded)-1]
		if _, isStr := last.(string); isStr {
			t.Errorf("last segment of removed path should be positional int, got string %q — publishDiff is using afterMap instead of before for removed paths", last)
		}
	}
}

// TestUpdateField_EmitsPositionalPath verifies that UpdateField publishes a
// positional integer-array path, not a raw dotted string.
func TestUpdateField_EmitsPositionalPath(t *testing.T) {
	pub := &capturePublisher{}
	store := newTestStoreWith(t, pub)

	g := makeTestGame(t, store, "updatefield")
	pub.reset()

	if err := store.UpdateField(g.ID.Hex(), "private", true); err != nil {
		t.Fatalf("UpdateField: %v", err)
	}

	ev := pub.last()
	if ev == nil {
		t.Fatal("no event published after UpdateField")
	}
	if len(ev.UpdatedFields) == 0 {
		t.Fatal("expected updated fields, got none")
	}

	assertPositionalPath(t, "UpdateField path", ev.UpdatedFields[0][0])
}

// TestDeleteField_EmitsPositionalPath verifies that DeleteField publishes a
// positional integer-array removed path, not a raw string.
func TestDeleteField_EmitsPositionalPath(t *testing.T) {
	pub := &capturePublisher{}
	store := newTestStoreWith(t, pub)

	g := makeTestGame(t, store, "deletefield")
	gameId := g.ID.Hex()

	// Set a custom field so we have something safe to delete.
	if err := store.UpdateField(gameId, "private", false); err != nil {
		t.Fatalf("UpdateField setup: %v", err)
	}

	pub.reset()

	if err := store.DeleteField(gameId, "private"); err != nil {
		t.Fatalf("DeleteField: %v", err)
	}

	ev := pub.last()
	if ev == nil {
		t.Fatal("no event published after DeleteField")
	}
	if len(ev.RemovedFields) == 0 {
		t.Fatal("expected removed fields, got none")
	}

	assertPositionalPath(t, "DeleteField removed path", ev.RemovedFields[0])
}

// TestMarkPlayerConnected_EmitsPositionalPath verifies that ConnectPlayer/
// DisconnectPlayer publishes a positional integer-array path.
func TestMarkPlayerConnected_EmitsPositionalPath(t *testing.T) {
	pub := &capturePublisher{}
	store := newTestStoreWith(t, pub)

	g := makeTestGame(t, store, "connected-path")

	slotId, err := store.AssignSlot(g.ID.Hex(), "A", "user-connect-1", "Alice")
	if err != nil {
		t.Fatalf("AssignSlot: %v", err)
	}

	pub.reset()

	if err := store.ConnectPlayer(g.ID.Hex(), slotId); err != nil {
		t.Fatalf("ConnectPlayer: %v", err)
	}

	ev := pub.last()
	if ev == nil {
		t.Fatal("no event published after ConnectPlayer")
	}
	if len(ev.UpdatedFields) == 0 {
		t.Fatal("expected updated fields, got none")
	}

	assertPositionalPath(t, "ConnectPlayer path", ev.UpdatedFields[0][0])
}
