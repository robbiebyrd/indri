package game

import (
	"context"
	"log"
	"time"

	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/services/events"
)

// changePublisher turns committed game writes into broadcast deltas. Every
// store reports each committed Mutate here, so what players receive never
// depends on the backend.
//
// Publish failures are logged, never returned: the write already committed,
// so a fan-out hiccup must not fail the mutation.
type changePublisher struct {
	ctx       context.Context
	publisher events.Publisher
}

// diff publishes the delta between the pre-write snapshot and the saved game,
// with paths positionally encoded against the game's own key order.
func (p changePublisher) diff(id string, before map[string]interface{}, after *models.Game) {
	if p.publisher == nil {
		return
	}

	afterMap, err := events.ToMap(after)
	if err != nil {
		log.Printf("could not snapshot game %v for change event: %v", id, err)
		return
	}

	updatedMap, removedPaths := events.Diff(before, afterMap)

	// Sanitize on string paths first, then encode to positional indices.
	rawUpdated := make([][]interface{}, 0, len(updatedMap))
	for k, v := range updatedMap {
		rawUpdated = append(rawUpdated, []interface{}{k, v})
	}
	rawRemoved := make([]interface{}, 0, len(removedPaths))
	for _, r := range removedPaths {
		rawRemoved = append(rawRemoved, r)
	}
	sanitized, sanitizedRemoved := events.SanitizeDelta(rawUpdated, rawRemoved)

	afterPos := events.BuildPositionalMap(afterMap)
	updated := make([][]interface{}, 0, len(sanitized))
	for _, pair := range sanitized {
		updated = append(updated, []interface{}{events.EncodePath(pair[0].(string), afterMap, afterPos), pair[1]})
	}

	// A removed key is absent from the after-state, so its position comes
	// from the before-state schema.
	beforePos := events.BuildPositionalMap(before)
	removed := make([]interface{}, 0, len(sanitizedRemoved))
	for _, r := range sanitizedRemoved {
		removed = append(removed, events.EncodePath(r.(string), before, beforePos))
	}

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
