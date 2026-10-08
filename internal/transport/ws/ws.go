// Package ws implements transport.Transport over WebSockets using
// coder/websocket. It is the only place that library is referenced outside
// tests; everything above the transport interface is protocol-agnostic.
package ws

import (
	"context"
	"errors"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	envVars "github.com/robbiebyrd/indri/internal/repo/env"
	"github.com/robbiebyrd/indri/internal/transport"
)

// path is the HTTP route the WebSocket upgrade is served on.
const path = "/ws"

// Transport is a WebSocket transport. The open-connection set and fan-out come
// from the embedded Registry, shared with every other adapter; this package
// supplies the upgrade and each connection's read, write and ping loops.
type Transport struct {
	*transport.Registry

	handlers transport.Handlers
	policy   *transport.OriginPolicy

	writeWait      time.Duration
	pongWait       time.Duration
	pingPeriod     time.Duration
	loginTimeout   time.Duration
	maxMessageSize int64
	bufferSize     int

	// bindMu serialises binding sockets to sessions, so two sockets binding
	// to one session at once cannot each close the other; see session.go.
	bindMu sync.Mutex
}

// conn is one WebSocket connection: per-connection keys and a buffered,
// drop-when-full outbox (BufferedConn), drained onto the socket by its own
// writer so a slow client never blocks a broadcast.
type conn struct {
	*transport.BufferedConn[[]byte]

	t  *Transport
	ws *websocket.Conn

	// closed is closed by the first close, so the connection's run notices a
	// server-side close (a kick, a shutdown) without waiting for the socket.
	// closeCode and closeReason are what that first close sends the client.
	closed      chan struct{}
	closeOnce   sync.Once
	closeCode   websocket.StatusCode
	closeReason string

	// deadline closes the socket if it is still bound to no session when it
	// fires; see session.go.
	mu       sync.Mutex
	deadline *time.Timer
}

func newConn(t *Transport, ws *websocket.Conn) *conn {
	return &conn{
		BufferedConn: transport.NewBufferedConn[[]byte]("", t.bufferSize),
		t:            t,
		ws:           ws,
		closed:       make(chan struct{}),
	}
}

// Close stops the connection taking new messages; what is already queued is
// still written, then the close frame.
func (c *conn) Close() error {
	return c.closeWith(websocket.StatusNormalClosure, "")
}

// closeWith is Close, sending the client code and reason in the close frame.
// Only the first close of a connection decides them.
func (c *conn) closeWith(code websocket.StatusCode, reason string) error {
	c.closeOnce.Do(func() {
		c.closeCode, c.closeReason = code, reason
		close(c.closed)
	})

	return c.BufferedConn.Close()
}

// New builds a WebSocket transport configured from the environment (timeouts,
// message size, and the origin allowlist for CSWSH protection).
func New() *Transport {
	vars := envVars.GetEnv()

	return &Transport{
		Registry:       &transport.Registry{},
		policy:         transport.NewOriginPolicy(vars.AllowedOrigins),
		writeWait:      time.Duration(vars.WSWriteTimeout) * time.Second,
		pongWait:       time.Duration(vars.WSPongTimeoutSeconds) * time.Second,
		pingPeriod:     time.Duration(vars.WSPingPeriodSeconds) * time.Second,
		loginTimeout:   time.Duration(vars.WSLoginTimeoutSeconds) * time.Second,
		maxMessageSize: int64(vars.WSMaxMessageSizeBytes),
		bufferSize:     vars.WSMessageBufferSize,
	}
}

func (t *Transport) Handle(h transport.Handlers) {
	t.handlers = h
}

func (t *Transport) Register(mux *http.ServeMux) {
	mux.HandleFunc(path, t.serve)
}

// serve upgrades one request and runs the connection until it ends.
func (t *Transport) serve(w http.ResponseWriter, r *http.Request) {
	if t.IsClosed() {
		http.Error(w, "websocket transport closed", http.StatusServiceUnavailable)

		return
	}

	// A WebSocket upgrade is not subject to CORS, so it consults the shared
	// origin policy directly instead of going through its middleware.
	if !t.policy.Allows(r) {
		log.Printf("websocket upgrade refused: origin %q is not allowed", r.Header.Get("Origin"))
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)

		return
	}

	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// The origin was checked above, by the policy every route shares;
		// the library's own check would apply a second, different rule.
		InsecureSkipVerify: true,

		// One deflate stream per connection, so each delta is compressed
		// against the ones before it. A client that does not offer
		// permessage-deflate is served plain frames.
		CompressionMode: websocket.CompressionContextTakeover,
	})
	if err != nil {
		// Accept has already answered the request.
		log.Printf("websocket upgrade failed (origin %q): %v", r.Header.Get("Origin"), err)

		return
	}

	// Counted on the inflated message, not the wire: a limit on compressed
	// bytes would let one small frame inflate to any size.
	ws.SetReadLimit(t.maxMessageSize)

	c := newConn(t, ws)

	t.run(r.Context(), c)
}

// run drives one connection until it ends: it is registered for fan-out, and
// its writer, pinger and read loop run until the socket fails or the conn is
// closed from this side.
//
// A close from this side ends the connection as soon as what was queued is
// flushed. The close handshake that follows waits for the client to answer,
// for up to five seconds, and a kicked client that is not reading must not hold
// up its own deregistration or the disconnect handler for that long.
func (t *Transport) run(ctx context.Context, c *conn) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	t.AddSink(c)
	c.startLoginDeadline()

	defer c.stopLoginDeadline()

	if t.handlers.Connect != nil {
		t.handlers.Connect(c)
	}

	flushed := make(chan struct{})
	go t.write(ctx, c, flushed)
	go t.ping(ctx, c)

	readDone := make(chan error, 1)
	go func() { readDone <- t.read(ctx, c) }()

	readEnded := false

	select {
	case err := <-readDone:
		readEnded = true

		if !c.IsClosed() && t.handlers.Error != nil && !closedNormally(err) {
			t.handlers.Error(c, err)
		}
	case <-c.closed:
	}

	t.RemoveSink(c)
	_ = c.Close()

	<-flushed

	if t.handlers.Disconnect != nil {
		t.handlers.Disconnect(c)
	}

	// Otherwise the read loop ends once the close handshake does.
	if !readEnded {
		<-readDone
	}
}

// read delivers every inbound message to the Message handler, one at a time,
// until the socket fails.
func (t *Transport) read(ctx context.Context, c *conn) error {
	for {
		_, msg, err := c.ws.Read(ctx)
		if err != nil {
			return err
		}

		if t.handlers.Message != nil {
			t.handlers.Message(c, msg)
		}
	}
}

// write drains the outbox onto the socket, closing flushed once the outbox is
// closed and empty, then closes the socket. A write that cannot finish within
// writeWait ends the connection: the client is gone or not reading.
func (t *Transport) write(ctx context.Context, c *conn, flushed chan<- struct{}) {
	for msg := range c.Events() {
		writeCtx, cancel := context.WithTimeout(ctx, t.writeWait)
		err := c.ws.Write(writeCtx, websocket.MessageText, msg)

		cancel()

		if err != nil {
			_ = c.Close()
			_ = c.ws.CloseNow()

			for range c.Events() {
				// Drain the rest; there is nowhere left to write it.
			}

			close(flushed)

			return
		}
	}

	close(flushed)

	_ = c.ws.Close(c.closeCode, c.closeReason)
}

// ping keeps an idle connection alive and notices a dead one: a ping that is
// not answered within pongWait closes the connection.
func (t *Transport) ping(ctx context.Context, c *conn) {
	ticker := time.NewTicker(t.pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pingCtx, cancel := context.WithTimeout(ctx, t.pongWait)
			err := c.ws.Ping(pingCtx)

			cancel()

			if err != nil {
				_ = c.ws.CloseNow()

				return
			}
		}
	}
}

// closedNormally reports whether a read ended because the connection was
// closed on purpose, by either side, rather than because something failed.
func closedNormally(err error) bool {
	switch websocket.CloseStatus(err) {
	case websocket.StatusNormalClosure, websocket.StatusGoingAway:
		return true
	}

	return errors.Is(err, context.Canceled)
}
