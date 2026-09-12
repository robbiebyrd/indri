// Package ws implements transport.Transport over WebSockets using melody.
// It is the only place melody is referenced; everything above the transport
// interface is protocol-agnostic.
package ws

import (
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/olahol/melody"

	envVars "github.com/robbiebyrd/indri/internal/repo/env"
	"github.com/robbiebyrd/indri/internal/transport"
)

// path is the HTTP route the WebSocket upgrade is served on.
const path = "/ws"

// Transport is a melody-backed WebSocket transport.
type Transport struct {
	m *melody.Melody
}

// conn adapts a melody session to transport.Conn.
type conn struct {
	s *melody.Session
}

func (c conn) Get(key string) (any, bool) { return c.s.Get(key) }
func (c conn) Set(key string, value any)  { c.s.Set(key, value) }
func (c conn) UnSet(key string)           { c.s.UnSet(key) }
func (c conn) Write(msg []byte) error     { return c.s.Write(msg) }
func (c conn) Close() error               { return c.s.Close() }
func (c conn) IsClosed() bool             { return c.s.IsClosed() }

// New builds a WebSocket transport configured from the environment (timeouts,
// message size, and the CheckOrigin allowlist for CSWSH protection).
func New() *Transport {
	vars := envVars.GetEnv()

	m := melody.New()

	m.Config = &melody.Config{
		WriteWait:                 time.Duration(vars.WSWriteTimeout) * time.Second,
		PongWait:                  time.Duration(vars.WSPongTimeoutSeconds) * time.Second,
		PingPeriod:                time.Duration(vars.WSPingPeriodSeconds) * time.Second,
		ConcurrentMessageHandling: false,
		MaxMessageSize:            int64(vars.WSMaxMessageSizeBytes),
		MessageBufferSize:         vars.WSMessageBufferSize,
	}

	// A WebSocket upgrade is not subject to CORS, so it consults the shared
	// origin policy directly instead of going through its middleware.
	m.Upgrader.CheckOrigin = transport.NewOriginPolicy(vars.AllowedOrigins).Allows

	return &Transport{m: m}
}

func (t *Transport) Handle(h transport.Handlers) {
	if h.Connect != nil {
		t.m.HandleConnect(func(s *melody.Session) { h.Connect(conn{s}) })
	}

	if h.Disconnect != nil {
		t.m.HandleDisconnect(func(s *melody.Session) { h.Disconnect(conn{s}) })
	}

	if h.Message != nil {
		t.m.HandleMessage(func(s *melody.Session, msg []byte) { h.Message(conn{s}, msg) })
	}

	if h.Error != nil {
		t.m.HandleError(func(s *melody.Session, err error) { h.Error(conn{s}, err) })
	}
}

func (t *Transport) Register(mux *http.ServeMux) {
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		err := t.m.HandleRequest(w, r)

		switch {
		case err == nil:
		case errors.Is(err, melody.ErrClosed):
			// Rejected before the upgrader touched the response writer.
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
		default:
			// A failed upgrade (bad handshake, disallowed Origin) has already
			// written its own status and body; writing again would only log a
			// superfluous WriteHeader warning.
			log.Printf("websocket upgrade failed (origin %q): %v", r.Header.Get("Origin"), err)
		}
	})
}

func (t *Transport) Broadcast(msg []byte) error {
	return t.m.Broadcast(msg)
}

func (t *Transport) BroadcastFilter(msg []byte, match func(transport.Conn) bool) error {
	return t.m.BroadcastFilter(msg, func(s *melody.Session) bool {
		return match(conn{s})
	})
}

func (t *Transport) Conns() ([]transport.Conn, error) {
	sessions, err := t.m.Sessions()
	if err != nil {
		return nil, err
	}

	conns := make([]transport.Conn, len(sessions))
	for i, s := range sessions {
		conns[i] = conn{s}
	}

	return conns, nil
}

func (t *Transport) Close() error {
	return t.m.Close()
}

func (t *Transport) IsClosed() bool {
	return t.m.IsClosed()
}
