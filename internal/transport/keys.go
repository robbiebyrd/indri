package transport

import "sync"

// SessionIDKey is the one per-connection key the application sets, on every
// transport. Its value is the session ObjectID (session.ID), never the
// bearer token the client holds — see the identity table in CLAUDE.md. Targeted
// broadcasts resolve their recipients through the session store and then filter
// connections on this key, so a connection without it is unreachable.
const SessionIDKey = "sessionId"

// Keys is the per-connection key/value state and closed flag shared by
// push-only connections (a GraphQL subscription sink, an SSE stream). Embed it
// to satisfy the Get/Set/UnSet/IsClosed half of Conn; the embedder supplies
// Write and Close, which differ per protocol.
//
// The zero value is not usable; build one with NewKeys.
type Keys struct {
	mu     sync.RWMutex
	keys   map[string]any
	closed bool
}

// NewKeys returns connection state carrying sessionID under SessionIDKey, so
// the connection is reachable by broadcast from the moment it is registered.
//
// An empty sessionID leaves the key unset rather than storing "": an
// anonymous connection (e.g. a WebRTC peer before login binds a session)
// must carry no session key at all, or a broadcast filter comparing against
// the empty string could match it.
func NewKeys(sessionID string) *Keys {
	keys := make(map[string]any)
	if sessionID != "" {
		keys[SessionIDKey] = sessionID
	}

	return &Keys{keys: keys}
}

func (k *Keys) Get(key string) (any, bool) {
	k.mu.RLock()
	defer k.mu.RUnlock()

	v, ok := k.keys[key]

	return v, ok
}

func (k *Keys) Set(key string, value any) {
	k.mu.Lock()
	defer k.mu.Unlock()

	k.keys[key] = value
}

func (k *Keys) UnSet(key string) {
	k.mu.Lock()
	defer k.mu.Unlock()

	delete(k.keys, key)
}

func (k *Keys) IsClosed() bool {
	k.mu.RLock()
	defer k.mu.RUnlock()

	return k.closed
}

// MarkClosed flips the closed flag and reports whether this call was the one
// that flipped it. The embedder releases its channel or stream only on a true
// return, so a double Close cannot close a channel twice (which panics).
//
// It takes the write lock, so it cannot overlap a WhileOpen body.
func (k *Keys) MarkClosed() bool {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.closed {
		return false
	}

	k.closed = true

	return true
}

// WhileOpen runs fn if the connection is still open, holding the read lock for
// the duration, and reports whether fn ran. Delivering to a push connection
// must go through it: MarkClosed takes the write lock, so a send started here
// cannot overlap the Close that releases the channel or stream it sends to.
//
// fn must not block indefinitely — it holds off Close for as long as it runs.
func (k *Keys) WhileOpen(fn func()) bool {
	k.mu.RLock()
	defer k.mu.RUnlock()

	if k.closed {
		return false
	}

	fn()

	return true
}
