package ws

import (
	"time"

	"github.com/coder/websocket"

	"github.com/robbiebyrd/indri/internal/transport"
)

// Close codes this transport ends a socket with, beyond the standard ones. A
// client tells them apart from a dropped connection by the code alone.
const (
	// StatusSuperseded closes a socket whose session a newer socket of the
	// same user has taken. A user holds one WebSocket at a time; the client
	// must not redial, or it would take the session back from the newer one.
	StatusSuperseded websocket.StatusCode = 4001

	// StatusLoginDeadline closes a socket that was bound to no session when
	// its login deadline passed. The client may redial when it next has
	// something to send.
	StatusLoginDeadline websocket.StatusCode = 4002
)

// Set is Conn.Set, except that binding the socket to a session goes through
// bind.
func (c *conn) Set(key string, value any) {
	if key != transport.SessionIDKey {
		c.BufferedConn.Set(key, value)

		return
	}

	c.t.bind(c, value)
}

// UnSet is Conn.UnSet. A socket unbound from its session is anonymous again, so
// its login deadline starts over.
func (c *conn) UnSet(key string) {
	c.BufferedConn.UnSet(key)

	if key == transport.SessionIDKey {
		c.startLoginDeadline()
	}
}

// bind binds c to session, and closes every other socket bound to the same
// session with StatusSuperseded: a user holds one WebSocket, and the newer
// socket keeps it. Sessions are one per user, so a session names one user.
//
// It runs under bindMu, so of several sockets binding to one session at once
// exactly one is left open, and a socket already closed binds nothing — a
// message it was sent before it was replaced must not take the session back.
func (t *Transport) bind(c *conn, session any) {
	t.bindMu.Lock()
	defer t.bindMu.Unlock()

	if c.IsClosed() {
		return
	}

	c.BufferedConn.Set(transport.SessionIDKey, session)
	c.stopLoginDeadline()

	conns, _ := t.Conns()

	for _, other := range conns {
		peer, ok := other.(*conn)
		if !ok || peer == c {
			continue
		}

		if bound, ok := peer.Get(transport.SessionIDKey); ok && bound == session {
			_ = peer.closeWith(StatusSuperseded, "the session moved to a newer connection")
		}
	}
}

// startLoginDeadline closes c with StatusLoginDeadline if it is still bound to
// no session once loginTimeout has passed, replacing any deadline already set.
// An anonymous socket costs the server its buffers and compression state, and
// a client that has not logged in has given nothing for them.
func (c *conn) startLoginDeadline() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.deadline != nil {
		c.deadline.Stop()
	}

	c.deadline = time.AfterFunc(c.t.loginTimeout, func() {
		// Under bindMu, so a login arriving as the deadline fires either
		// binds first and is kept, or binds nothing on a closed socket.
		c.t.bindMu.Lock()
		defer c.t.bindMu.Unlock()

		if _, bound := c.Get(transport.SessionIDKey); !bound {
			_ = c.closeWith(StatusLoginDeadline, "no login before the deadline")
		}
	})
}

func (c *conn) stopLoginDeadline() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.deadline != nil {
		c.deadline.Stop()
		c.deadline = nil
	}
}
