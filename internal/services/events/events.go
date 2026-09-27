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

type OpCode uint8

const (
	OpUpdate OpCode = 1
	OpInsert OpCode = 2
	OpDelete OpCode = 3
	// OpLayout marks a layout frame on the wire. On the change bus it reports
	// that a write edited the game's layout, so the broadcaster sends the new one.
	OpLayout OpCode = 4
	// OpKeyframe is server-internal: it asks the broadcaster to send the game's
	// clients a fresh keyframe, because a write changed the document's shape and a
	// positional delta would no longer decode against their schema.
	OpKeyframe OpCode = 5
)

// ChangeEvent is the delta broadcast to clients. UpdatedFields is a slice of
// [path, value] pairs. RemovedFields is a slice of paths.
type ChangeEvent struct {
	ID            string          `json:"id"`
	OperationType OpCode          `json:"o"`
	Timestamp     time.Time       `json:"t"`
	Collection    string          `json:"type,omitempty"`
	UpdatedFields [][]interface{} `json:"u,omitempty"`
	RemovedFields []interface{}   `json:"r,omitempty"`
}

// HasChanges reports whether the event carries any field change worth
// broadcasting.
func (e ChangeEvent) HasChanges() bool {
	return len(e.UpdatedFields) > 0 || len(e.RemovedFields) > 0
}

// Bus fans messages of one kind out to every server instance. Subscribe yields
// messages published by any instance, this one included.
type Bus[T any] interface {
	Publish(ctx context.Context, message T) error
	Subscribe(ctx context.Context) (<-chan T, error)
}

// Publisher fans change events out to every server instance. Publish is called
// at each write; Subscribe yields events for the broadcast loop to deliver to
// each instance's clients.
type Publisher = Bus[ChangeEvent]

// Delivery is a message for specific sessions' connections, wherever in the
// cluster they are connected. Close disconnects them instead of sending
// Payload.
type Delivery struct {
	SessionIDs []string `json:"sessionIds"`
	Payload    any      `json:"payload,omitempty"`
	Close      bool     `json:"close,omitempty"`
}
