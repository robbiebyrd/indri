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

func TestChangePublisher_DiffPublishesSanitizedDelta(t *testing.T) {
	pub := &recordingPublisher{}
	cp := changePublisher{ctx: context.Background(), publisher: pub}

	before := &models.Game{Code: "G1", PublicData: map[string]interface{}{"n": 1.0}}
	after := &models.Game{
		Code:        "G1",
		PublicData:  map[string]interface{}{"n": 2.0},
		PrivateData: map[string]interface{}{"secret": "x"},
	}

	beforeMap, err := events.ToMap(before)
	if err != nil {
		t.Fatal(err)
	}

	cp.diff("g1", beforeMap, after)

	if len(pub.events) != 1 {
		t.Fatalf("published %d events, want 1", len(pub.events))
	}

	ev := pub.events[0]
	if ev.ID != "g1" || ev.OperationType != events.OpUpdate || ev.Collection != collectionName {
		t.Errorf("event header = %+v", ev)
	}
	if ev.UpdatedFields["data.n"] != 2.0 {
		t.Errorf("data.n = %v, want 2", ev.UpdatedFields["data.n"])
	}
	for k := range ev.UpdatedFields {
		if k == "privateData" || k == "privateData.secret" {
			t.Errorf("private data leaked into the delta: %v", k)
		}
	}
}

func TestChangePublisher_FieldAndNoChangeAndNilPublisher(t *testing.T) {
	pub := &recordingPublisher{}
	cp := changePublisher{ctx: context.Background(), publisher: pub}

	cp.field("g1", map[string]interface{}{"data.x": 1}, nil)
	cp.field("g1", nil, []string{"data.y"})
	cp.field("g1", nil, nil)

	if len(pub.events) != 2 {
		t.Fatalf("published %d events, want 2 (a no-change write publishes nothing)", len(pub.events))
	}
	if pub.events[1].RemovedFields[0] != "data.y" {
		t.Errorf("removed = %v", pub.events[1].RemovedFields)
	}

	changePublisher{ctx: context.Background()}.field("g1", map[string]interface{}{"data.x": 1}, nil)
}

// A publish failure must not fail the write that already committed.
func TestChangePublisher_PublishErrorIsNotFatal(t *testing.T) {
	pub := &recordingPublisher{err: errors.New("bus down")}
	cp := changePublisher{ctx: context.Background(), publisher: pub}

	cp.field("g1", map[string]interface{}{"data.x": 1}, nil)

	if len(pub.events) != 1 {
		t.Fatalf("published %d events, want 1", len(pub.events))
	}
}
