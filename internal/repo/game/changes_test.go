package game

import (
	"context"
	"errors"
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
