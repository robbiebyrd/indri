package transport

import (
	"slices"
	"sync"
)

// Registry is the set of push-only connections a transport currently holds
// open, plus the fan-out over them. It implements every part of Transport that
// does not depend on the wire protocol; a transport embeds it and supplies only
// Handle and Register.
//
// The zero value is ready to use.
type Registry struct {
	mu     sync.RWMutex
	sinks  map[Conn]struct{}
	peer   Transport
	closed bool
}

// SetPeer wires the aggregate transport, so Disconnect can reach a session
// connected over a different protocol. Without it, Disconnect sees only this
// transport's own connections.
func (r *Registry) SetPeer(p Transport) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.peer = p
}

// AddSink registers a connection for fan-out. A connection offered after Close
// is closed instead of registered: the transport is shutting down and nothing
// would come along to close it later.
func (r *Registry) AddSink(c Conn) {
	r.mu.Lock()

	if r.closed {
		r.mu.Unlock()
		_ = c.Close()

		return
	}

	if r.sinks == nil {
		r.sinks = make(map[Conn]struct{})
	}

	r.sinks[c] = struct{}{}
	r.mu.Unlock()
}

// RemoveSink deregisters a connection.
func (r *Registry) RemoveSink(c Conn) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.sinks, c)
}

// Disconnect closes every connection whose session is in sessionIDs, across all
// transports when a peer is wired. Used by kick and logout.
func (r *Registry) Disconnect(sessionIDs []string) {
	r.mu.RLock()
	target := r.peer
	r.mu.RUnlock()

	var (
		conns []Conn
		err   error
	)

	if target != nil {
		conns, err = target.Conns()
	} else {
		conns, err = r.Conns()
	}

	if err != nil {
		return
	}

	for _, c := range conns {
		value, ok := c.Get(SessionIDKey)
		if !ok {
			continue
		}

		if id, ok := value.(string); ok && slices.Contains(sessionIDs, id) {
			_ = c.Close()
		}
	}
}

func (r *Registry) Broadcast(msg []byte) error {
	return r.BroadcastFilter(msg, func(Conn) bool { return true })
}

// BroadcastFilter writes msg to each matching connection. A per-connection
// write error is not returned: one stalled client must not abort fan-out to
// everybody else, and the connection's own read loop will notice the failure.
func (r *Registry) BroadcastFilter(msg []byte, match func(Conn) bool) error {
	for _, c := range r.snapshot() {
		if match(c) {
			_ = c.Write(msg)
		}
	}

	return nil
}

func (r *Registry) Conns() ([]Conn, error) {
	return r.snapshot(), nil
}

// snapshot copies the connection set so fan-out runs without holding the lock,
// which a Write may otherwise re-enter.
func (r *Registry) snapshot() []Conn {
	r.mu.RLock()
	defer r.mu.RUnlock()

	conns := make([]Conn, 0, len(r.sinks))
	for c := range r.sinks {
		conns = append(conns, c)
	}

	return conns
}

func (r *Registry) Close() error {
	r.mu.Lock()

	r.closed = true
	sinks := r.sinks
	r.sinks = nil

	r.mu.Unlock()

	for c := range sinks {
		_ = c.Close()
	}

	return nil
}

func (r *Registry) IsClosed() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.closed
}
