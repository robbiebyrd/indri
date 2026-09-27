package game

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/services/events"
)

type recordingPublisher struct {
	events []events.ChangeEvent
	err    error
}

func (p *recordingPublisher) Publish(_ context.Context, e events.ChangeEvent) error {
	p.events = append(p.events, e)
	return p.err
}

func (p *recordingPublisher) Subscribe(context.Context) (<-chan events.ChangeEvent, error) {
	return nil, nil
}

func snapshot(t *testing.T, g *models.Game) map[string]interface{} {
	t.Helper()

	m, err := events.ToMap(g)
	if err != nil {
		t.Fatal(err)
	}

	return m
}

func TestChangePublisher_DiffPublishesSanitizedPositionalDelta(t *testing.T) {
	pub := &recordingPublisher{}
	cp := changePublisher{ctx: context.Background(), publisher: pub}

	before := &models.Game{Code: "G1", PublicData: map[string]interface{}{"n": 1.0}}
	after := &models.Game{
		Code:        "G1",
		PublicData:  map[string]interface{}{"n": 2.0},
		PrivateData: map[string]interface{}{"secret": "x"},
	}

	cp.diff("g1", snapshot(t, before), after)

	if len(pub.events) != 1 {
		t.Fatalf("published %d events, want 1", len(pub.events))
	}

	ev := pub.events[0]
	if ev.ID != "g1" || ev.OperationType != events.OpUpdate || ev.Collection != collectionName {
		t.Errorf("event header = %+v", ev)
	}
	if len(ev.UpdatedFields) != 1 {
		t.Fatalf("updated = %v, want only data.n (private data must be stripped)", ev.UpdatedFields)
	}

	path, ok := ev.UpdatedFields[0][0].([]interface{})
	if !ok || len(path) != 2 {
		t.Fatalf("path = %#v, want a two-segment positional path", ev.UpdatedFields[0][0])
	}
	if _, isInt := path[0].(int); !isInt {
		t.Errorf("first segment %#v is not positional", path[0])
	}
	if ev.UpdatedFields[0][1] != 2.0 {
		t.Errorf("value = %v, want 2", ev.UpdatedFields[0][1])
	}
}

func TestChangePublisher_NoChangeAndNilPublisherPublishNothing(t *testing.T) {
	pub := &recordingPublisher{}
	g := &models.Game{Code: "G1"}

	changePublisher{ctx: context.Background(), publisher: pub}.diff("g1", snapshot(t, g), g)
	if len(pub.events) != 0 {
		t.Fatalf("published %d events for an unchanged game", len(pub.events))
	}

	changePublisher{ctx: context.Background()}.diff("g1", snapshot(t, g), &models.Game{Code: "G2"})
}

// A publish failure must not fail the write that already committed.
func TestChangePublisher_PublishErrorIsNotFatal(t *testing.T) {
	pub := &recordingPublisher{err: errors.New("bus down")}
	cp := changePublisher{ctx: context.Background(), publisher: pub}

	cp.diff("g1", snapshot(t, &models.Game{Code: "A"}), &models.Game{Code: "B"})

	if len(pub.events) != 1 {
		t.Fatalf("published %d events, want 1", len(pub.events))
	}
}

// Deltas are encoded against the client's view of the game, so a path
// decodes to the right key even when private data or the layout (which the
// client never receives) sort before it.
func TestChangePublisher_PositionsMatchTheClientView(t *testing.T) {
	pub := &recordingPublisher{}
	cp := changePublisher{ctx: context.Background(), publisher: pub}

	before := &models.Game{
		Code:        "G1",
		PrivateData: map[string]interface{}{"secret": "x"},
		PublicData:  map[string]interface{}{"layout": map[string]interface{}{"g": 1}, "turn": "x"},
		Stage:       models.Stage{CurrentScene: "lobby"},
	}
	after := &models.Game{
		Code:        "G1",
		PrivateData: map[string]interface{}{"secret": "x"},
		PublicData:  map[string]interface{}{"layout": map[string]interface{}{"g": 1}, "turn": "o"},
		Stage:       models.Stage{CurrentScene: "board"},
	}

	cp.diff("g1", snapshot(t, before), after)

	if len(pub.events) != 1 || pub.events[0].OperationType != events.OpUpdate {
		t.Fatalf("events = %+v, want one update", pub.events)
	}

	view := events.ClientView(snapshot(t, after))
	got := map[string]interface{}{}
	for _, pair := range pub.events[0].UpdatedFields {
		got[resolveAgainst(t, pair[0].([]interface{}), view)] = pair[1]
	}

	want := map[string]interface{}{"stage.currentScene": "board", "data.turn": "o"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("client would apply %v, want %v", got, want)
	}
}

// A write that adds or removes a key changes positions the client's schema
// can't know, so the client gets a fresh keyframe instead of a delta.
func TestChangePublisher_ShapeChangeRequestsAKeyframe(t *testing.T) {
	pub := &recordingPublisher{}
	cp := changePublisher{ctx: context.Background(), publisher: pub}

	before := &models.Game{Code: "G1", PublicData: map[string]interface{}{"b": 1.0}}
	after := &models.Game{Code: "G1", PublicData: map[string]interface{}{"a": 1.0, "b": 1.0}}

	cp.diff("g1", snapshot(t, before), after)

	if len(pub.events) != 1 {
		t.Fatalf("published %d events, want 1", len(pub.events))
	}
	ev := pub.events[0]
	if ev.OperationType != events.OpKeyframe || ev.ID != "g1" || ev.HasChanges() {
		t.Fatalf("event = %+v, want a bare keyframe request for g1", ev)
	}
}

// Private data never reaches clients, so changing only it publishes nothing,
// even when it adds keys.
func TestChangePublisher_PrivateOnlyChangePublishesNothing(t *testing.T) {
	pub := &recordingPublisher{}
	cp := changePublisher{ctx: context.Background(), publisher: pub}

	before := &models.Game{Code: "G1", PrivateData: map[string]interface{}{"a": 1.0}}
	after := &models.Game{Code: "G1", PrivateData: map[string]interface{}{"a": 2.0, "b": 1.0}}

	cp.diff("g1", snapshot(t, before), after)

	if len(pub.events) != 0 {
		t.Fatalf("published %+v for a private-only change", pub.events)
	}
}

// resolveAgainst decodes a positional path the way the client does
// (client/services/positional-map.ts).
func resolveAgainst(t *testing.T, path []interface{}, schema interface{}) string {
	t.Helper()

	var parts []string
	current := schema

	for _, seg := range path {
		obj, ok := current.(map[string]interface{})
		idx, isInt := seg.(int)
		if !ok || !isInt {
			t.Fatalf("path %v is not decodable against the client schema", path)
		}

		keys := make([]string, 0, len(obj))
		for k := range obj {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		parts = append(parts, keys[idx])
		current = obj[keys[idx]]
	}

	return strings.Join(parts, ".")
}
