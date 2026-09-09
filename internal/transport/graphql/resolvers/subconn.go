package resolvers

import (
	"sync"

	"github.com/robbiebyrd/indri/internal/transport/graphql/model"
)

// subConn adapts a subscription's event channel to transport.Conn, so the
// existing broadcast fan-out (which matches on the "sessionId" key and calls
// Write) drives GraphQL subscribers exactly like WebSocket clients. Write
// pushes a delta onto the channel the subscription resolver returned.
type subConn struct {
	ch     chan model.JSON
	mu     sync.RWMutex
	keys   map[string]any
	closed bool
}

func newSubConn(sessionID string) *subConn {
	return &subConn{
		ch:   make(chan model.JSON, 16),
		keys: map[string]any{"sessionId": sessionID},
	}
}

func (c *subConn) events() <-chan model.JSON { return c.ch }

func (c *subConn) Get(key string) (any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	v, ok := c.keys[key]

	return v, ok
}

func (c *subConn) Set(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.keys[key] = value
}

func (c *subConn) UnSet(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.keys, key)
}

func (c *subConn) Write(msg []byte) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.closed {
		return nil
	}

	// Copy the bytes: the caller may reuse its buffer, and the value outlives
	// this call on the channel.
	payload := make(model.JSON, len(msg))
	copy(payload, msg)

	select {
	case c.ch <- payload:
	default:
		// Slow subscriber; drop rather than block the broadcaster.
	}

	return nil
}

func (c *subConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	c.closed = true
	close(c.ch)

	return nil
}

func (c *subConn) IsClosed() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.closed
}
