// Package sse implements transport.Transport over Server-Sent Events for
// server-to-client messages and plain HTTP POSTs for client-to-server ones.
//
// Wire format:
//
//	GET  /sse/stream   opens the stream. The first event is
//	                   "event: connected" whose data is the connection ID.
//	                   Text messages follow as default events; binary ones as
//	                   "event: binary" with base64 data. ": ping" comments
//	                   keep idle streams alive.
//	POST /sse/send     body is one message; the X-Indri-Connection-Id header
//	                   names the stream it belongs to. Responds 204 once the
//	                   message has been handled, 404 for an unknown or closed
//	                   connection (the client should reconnect).
package sse

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/robbiebyrd/indri/internal/transport"
)

const (
	streamPath = "/sse/stream"
	sendPath   = "/sse/send"
)

// Config tunes the transport.
type Config struct {
	// AllowedOrigins is the comma-separated browser Origin allowlist.
	AllowedOrigins string
	// BufferSize is how many outbound messages may queue per connection.
	BufferSize int
	// MaxMessageSize caps a POSTed message, in bytes.
	MaxMessageSize int64
	// PingPeriod is the keepalive interval on idle streams.
	PingPeriod time.Duration
}

// Transport is an SSE + POST transport.
type Transport struct {
	*transport.Hub
	cfg Config
}

func New(cfg Config) *Transport {
	return &Transport{Hub: transport.NewHub(), cfg: cfg}
}

func (t *Transport) Register(mux *http.ServeMux) {
	transport.Route(mux, t.cfg.AllowedOrigins, http.MethodGet, streamPath, http.HandlerFunc(t.stream))
	transport.Route(mux, t.cfg.AllowedOrigins, http.MethodPost, sendPath, http.HandlerFunc(t.send))
}

// stream owns the response for the life of the connection and is the only
// goroutine that writes to it.
func (t *Transport) stream(w http.ResponseWriter, r *http.Request) {
	rc := http.NewResponseController(w)

	// The server's Read/WriteTimeouts are sized for ordinary requests and
	// would cut every stream off; this response is meant to stay open.
	if err := rc.SetReadDeadline(time.Time{}); err != nil {
		log.Printf("sse: clearing read deadline: %v", err)
	}
	if err := rc.SetWriteDeadline(time.Time{}); err != nil {
		log.Printf("sse: clearing write deadline: %v", err)
	}

	id, err := transport.NewConnectionID()
	if err != nil {
		http.Error(w, "could not open stream", http.StatusInternalServerError)
		return
	}

	c := transport.NewQueuedConn(t.cfg.BufferSize, nil)
	if err := t.Hub.Add(id, c); err != nil {
		http.Error(w, "server shutting down", http.StatusServiceUnavailable)
		return
	}

	handlers := t.Handlers()

	defer func() {
		t.Hub.Remove(id)
		_ = c.Close()
		c.Disconnected(handlers)
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	emit := func(chunk string) bool {
		if _, err := io.WriteString(w, chunk); err != nil {
			return false
		}

		return rc.Flush() == nil
	}

	// The client must hold its ID before Connect runs, because Connect
	// immediately writes the first message.
	if !emit(event("connected", id)) {
		return
	}

	c.Connected(handlers)

	ping := time.NewTicker(t.cfg.PingPeriod)
	defer ping.Stop()

	for {
		select {
		case f := <-c.Outbound():
			if !emit(encode(f)) {
				return
			}
		case <-ping.C:
			if !emit(": ping\n\n") {
				return
			}
		case <-c.Done():
			c.Drain(func(f transport.Frame) bool { return emit(encode(f)) })
			return
		case <-r.Context().Done():
			return
		}
	}
}

func (t *Transport) send(w http.ResponseWriter, r *http.Request) {
	c, ok := t.Hub.Lookup(r.Header.Get(transport.ConnectionIDHeader))
	if !ok || c.IsClosed() {
		http.Error(w, "unknown connection", http.StatusNotFound)
		return
	}

	msg, err := io.ReadAll(http.MaxBytesReader(w, r.Body, t.cfg.MaxMessageSize))
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			http.Error(w, "message too large", http.StatusRequestEntityTooLarge)
			return
		}

		http.Error(w, "could not read message", http.StatusBadRequest)

		return
	}

	// Responding only after the handler returns lets a client that awaits
	// each send keep its messages in order.
	c.Deliver(t.Handlers(), msg)

	w.WriteHeader(http.StatusNoContent)
}

func encode(f transport.Frame) string {
	if f.Binary {
		return event("binary", base64.StdEncoding.EncodeToString(f.Data))
	}

	return event("", string(f.Data))
}

// event formats one SSE event. A newline inside data would end the field, so
// each line becomes its own data field, which the client rejoins with "\n".
func event(name, data string) string {
	var b strings.Builder

	if name != "" {
		fmt.Fprintf(&b, "event: %s\n", name)
	}

	for _, line := range strings.Split(data, "\n") {
		fmt.Fprintf(&b, "data: %s\n", line)
	}

	b.WriteString("\n")

	return b.String()
}
