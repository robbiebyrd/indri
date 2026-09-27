package game

import (
	"context"
	"log"
	"time"

	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/services/events"
)

// changePublisher turns committed game writes into broadcast deltas. Every
// store delegates to it so what players receive never depends on the backend;
// a store only reports what it wrote.
//
// Publish failures are logged, never returned: the write already committed,
// so a fan-out hiccup must not fail the mutation.
type changePublisher struct {
	ctx       context.Context
	publisher events.Publisher
}

// diff publishes the delta between the pre-write snapshot and the saved game.
func (p changePublisher) diff(id string, before map[string]interface{}, after *models.Game) {
	if p.publisher == nil {
		return
	}

	afterMap, err := events.ToMap(after)
	if err != nil {
		log.Printf("could not snapshot game %v for change event: %v", id, err)
		return
	}

	updated, removed := events.Diff(before, afterMap)

	p.field(id, updated, removed)
}

// field publishes dotted-path changes the store made directly.
func (p changePublisher) field(id string, updated map[string]interface{}, removed []string) {
	if p.publisher == nil {
		return
	}

	// Strip private data so a broadcast delta never exposes more than a
	// sanitized keyframe would.
	updated, removed = events.SanitizeDelta(updated, removed)

	event := events.ChangeEvent{
		ID:            id,
		OperationType: events.OpUpdate,
		Timestamp:     time.Now(),
		Collection:    collectionName,
		UpdatedFields: updated,
		RemovedFields: removed,
	}

	if !event.HasChanges() {
		return
	}

	if err := p.publisher.Publish(p.ctx, event); err != nil {
		log.Printf("could not publish change event for game %v: %v", id, err)
	}
}
