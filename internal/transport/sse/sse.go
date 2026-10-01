// Package sse implements the server -> client half of transport.Transport over
// Server-Sent Events, mounted at /events. It is push-only by design: SSE has no
// inbound channel, so clients send actions over the REST action API
// (internal/transport/rest) or as GraphQL mutations, both of which dispatch to
// the same connection-independent action logic.
//
// A stream is authenticated at the handshake and carries the session key from
// the moment it registers, so the existing broadcast fan-out reaches it exactly
// like a WebSocket client or a GraphQL subscriber.
package sse

import (
	"bytes"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/transport"
)

// path is the HTTP route the event stream is served on.
const path = "/events"

// streamBuffer is how many deltas a subscriber may fall behind by before its
// oldest are dropped. It matches the GraphQL subscription buffer.
const streamBuffer = 16

// defaultHeartbeat is how often an idle stream emits a comment frame. Proxies
// commonly drop an idle connection at 60s, so stay well inside that.
const defaultHeartbeat = 25 * time.Second

// SessionLookup resolves a session from its bearer token (the same opaque token
// login and reconnect issue). Satisfied by the session store.
type SessionLookup interface {
	GetByToken(token string) (*models.Session, error)
}

// Transport is the SSE transport. The set of open streams and the fan-out over
// them come from the embedded Registry, shared with the GraphQL transport.
type Transport struct {
	*transport.Registry

	sessions SessionLookup

	// Heartbeat is the idle keep-alive interval. Set it before Register.
	Heartbeat time.Duration
}

// New builds the SSE transport. sessions authenticates each stream.
func New(sessions SessionLookup) *Transport {
	return &Transport{
		Registry:  &transport.Registry{},
		sessions:  sessions,
		Heartbeat: defaultHeartbeat,
	}
}

// Handle is a no-op: an SSE stream carries no inbound messages, so there are no
// lifecycle callbacks to drive. Client actions arrive over REST or GraphQL.
func (t *Transport) Handle(_ transport.Handlers) {}

func (t *Transport) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET "+path, t.stream)
}

// stream authenticates the request, registers a push connection for it, and
// then blocks writing frames until the client goes away or the transport
// closes. It holds the request goroutine for the life of the stream, which is
// how SSE works.
func (t *Transport) stream(w http.ResponseWriter, r *http.Request) {
	session, err := t.sessions.GetByToken(tokenFrom(r))
	if err != nil || session == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)

		return
	}

	// The request's own write deadline (http.Server.WriteTimeout) would cut the
	// stream off mid-life. Unlike a WebSocket upgrade, SSE never hijacks the
	// connection, so it has to clear the deadline itself.
	rc := http.NewResponseController(w)
	if err := rc.SetWriteDeadline(time.Time{}); err != nil {
		log.Printf("sse: clearing the write deadline: %v", err)
	}

	header := w.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	// Tell nginx not to buffer the stream; without it frames arrive in batches.
	header.Set("X-Accel-Buffering", "no")

	w.WriteHeader(http.StatusOK)

	if err := rc.Flush(); err != nil {
		log.Printf("sse: flushing the stream headers: %v", err)

		return
	}

	conn := transport.NewBufferedConn[[]byte](session.ID, streamBuffer)

	t.AddSink(conn)
	defer func() {
		t.RemoveSink(conn)
		_ = conn.Close()
	}()

	// AddSink closes the conn instead of registering it when the transport is
	// already shutting down; nothing would drain the stream in that case.
	if conn.IsClosed() {
		return
	}

	heartbeat := time.NewTicker(t.heartbeatInterval())
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			// The client went away, or the server is shutting the request down.
			return

		case msg, open := <-conn.Events():
			if !open {
				// Close from the transport side: a kick, or shutdown.
				return
			}

			if err := writeFrame(w, rc, msg); err != nil {
				return
			}

		case <-heartbeat.C:
			// A comment frame. Clients ignore it; proxies count it as traffic.
			if err := writeRaw(w, rc, []byte(":ping\n\n")); err != nil {
				return
			}
		}
	}
}

func (t *Transport) heartbeatInterval() time.Duration {
	if t.Heartbeat <= 0 {
		return defaultHeartbeat
	}

	return t.Heartbeat
}

// tokenFrom reads the session token from the Authorization header, falling back
// to the token query parameter. The fallback is not a convenience: the browser
// EventSource API cannot set request headers, so it is the only way a web
// client can authenticate its stream.
func tokenFrom(r *http.Request) string {
	if auth := r.Header.Get("Authorization"); auth != "" {
		return strings.TrimPrefix(auth, "Bearer ")
	}

	return r.URL.Query().Get("token")
}

// writeFrame emits msg as one SSE data event. Every line is prefixed
// separately, because a bare newline inside the payload would otherwise end the
// event early and split one delta across two.
func writeFrame(w http.ResponseWriter, rc *http.ResponseController, msg []byte) error {
	var frame bytes.Buffer

	for _, line := range bytes.Split(msg, []byte("\n")) {
		frame.WriteString("data: ")
		frame.Write(bytes.TrimSuffix(line, []byte("\r")))
		frame.WriteString("\n")
	}

	frame.WriteString("\n")

	return writeRaw(w, rc, frame.Bytes())
}

func writeRaw(w http.ResponseWriter, rc *http.ResponseController, b []byte) error {
	if _, err := w.Write(b); err != nil {
		return err
	}

	return rc.Flush()
}
