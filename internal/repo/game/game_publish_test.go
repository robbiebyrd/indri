package game

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/services/events"
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

// newTestStoreWith returns an in-memory store: what these tests cover (slot
// operations and positional delta encoding) is shared by every backend.
func newTestStoreWith(t *testing.T, publisher events.Publisher) Storer {
	t.Helper()

	return newMemoryStoreWith(t, publisher)
}

func makeTestGame(t *testing.T, store Storer, prefix string) *models.Game {
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

	if err := store.UpdateField(g.ID, "data.list", []interface{}{"a", "b", "c"}); err != nil {
		t.Fatalf("setup: %v", err)
	}

	pub.reset()

	// Shrinking an array removes its tail without changing any object key, so
	// it stays a positional delta; the removed element exists only in the
	// before-state, so its path must be encoded from that.
	if err := store.UpdateField(g.ID, "data.list", []interface{}{"a", "b"}); err != nil {
		t.Fatalf("shrink: %v", err)
	}

	ev := pub.last()
	if ev == nil || ev.OperationType != events.OpUpdate || len(ev.RemovedFields) != 1 {
		t.Fatalf("event = %+v, want an update removing one element", ev)
	}

	encoded, ok := ev.RemovedFields[0].([]interface{})
	if !ok || len(encoded) != 3 {
		t.Fatalf("removed path = %#v, want [data, list, 2] positionally", ev.RemovedFields[0])
	}
	for i, seg := range encoded {
		if _, isInt := seg.(int); !isInt {
			t.Errorf("segment %d = %#v, want a positional int", i, seg)
		}
	}
	if encoded[2] != 2 {
		t.Errorf("removed index = %v, want 2", encoded[2])
	}
}

// TestUpdateField_EmitsPositionalPath verifies that UpdateField publishes a
// positional integer-array path, not a raw dotted string.
func TestUpdateField_EmitsPositionalPath(t *testing.T) {
	pub := &capturePublisher{}
	store := newTestStoreWith(t, pub)

	g := makeTestGame(t, store, "updatefield")
	pub.reset()

	// A value change on an existing key keeps the schema, so it is a delta.
	if err := store.UpdateField(g.ID, "players.p0.name", "Zed"); err != nil {
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

// TestDeleteField_RequestsKeyframe: deleting an object key shifts its
// siblings' positions, so clients get a fresh keyframe instead of a delta.
func TestDeleteField_RequestsKeyframe(t *testing.T) {
	pub := &capturePublisher{}
	store := newTestStoreWith(t, pub)

	g := makeTestGame(t, store, "deletefield")
	gameId := g.ID

	// Set a field clients can see, so deleting it is a visible change.
	if err := store.UpdateField(gameId, "data.scratch", "x"); err != nil {
		t.Fatalf("UpdateField setup: %v", err)
	}

	pub.reset()

	if err := store.DeleteField(gameId, "data.scratch"); err != nil {
		t.Fatalf("DeleteField: %v", err)
	}

	ev := pub.last()
	if ev == nil || ev.OperationType != events.OpKeyframe {
		t.Fatalf("event = %+v, want a keyframe request", ev)
	}
}

// TestMarkPlayerConnected_EmitsPositionalPath verifies that ConnectPlayer/
// DisconnectPlayer publishes a positional integer-array path.
func TestMarkPlayerConnected_EmitsPositionalPath(t *testing.T) {
	pub := &capturePublisher{}
	store := newTestStoreWith(t, pub)

	g := makeTestGame(t, store, "connected-path")

	slotId, err := store.AssignSlot(g.ID, "A", "user-connect-1", "Alice")
	if err != nil {
		t.Fatalf("AssignSlot: %v", err)
	}

	// AssignSlot already connects the player; disconnect so connecting is a change.
	if err := store.DisconnectPlayer(g.ID, slotId); err != nil {
		t.Fatalf("DisconnectPlayer: %v", err)
	}

	pub.reset()

	if err := store.ConnectPlayer(g.ID, slotId); err != nil {
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
