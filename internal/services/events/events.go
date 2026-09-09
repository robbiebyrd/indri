// Package events publishes record-change deltas computed in application code,
// replacing MongoDB change streams. Because the app is the sole writer, it can
// derive the delta from the before/after state at the write site and fan it
// out itself — no replica set, and it works with any backing store. The
// Publisher interface has an in-process implementation (single instance) and a
// Redis Pub/Sub implementation (multi-instance).
package events

import (
	"context"
	"time"
)

type OperationType string

const (
	OpUpdate OperationType = "update"
	OpInsert OperationType = "insert"
	OpDelete OperationType = "delete"
)

// ChangeEvent is the delta broadcast to clients. Its JSON shape matches what
// the client reducer consumes: dotted-path "updated" fields and "removed"
// paths.
type ChangeEvent struct {
	ID            string                 `json:"id"`
	OperationType OperationType          `json:"op"`
	Timestamp     time.Time              `json:"ts"`
	Collection    string                 `json:"type,omitempty"`
	UpdatedFields map[string]interface{} `json:"updated,omitempty"`
	RemovedFields []string               `json:"removed,omitempty"`
}

// HasChanges reports whether the event carries any field change worth
// broadcasting.
func (e ChangeEvent) HasChanges() bool {
	return len(e.UpdatedFields) > 0 || len(e.RemovedFields) > 0
}

// Publisher fans change events out to every server instance. Publish is called
// at each write; Subscribe yields events (local and, for Redis, from other
// instances) for the broadcast loop to deliver to websocket clients.
type Publisher interface {
	Publish(ctx context.Context, event ChangeEvent) error
	Subscribe(ctx context.Context) (<-chan ChangeEvent, error)
}
