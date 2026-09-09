// Package graphql implements transport.Transport over GraphQL: typed mutations
// (client -> server) dispatched to the shared action logic, and a gameUpdates
// subscription (server -> client) fed by the existing broadcast fan-out. It is
// mounted at /graphql and runs alongside the WebSocket transport.
package graphql

import (
	"context"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	gqlhttp "github.com/99designs/gqlgen/graphql/handler/transport"

	"github.com/robbiebyrd/indri/internal/transport"
	"github.com/robbiebyrd/indri/internal/transport/graphql/generated"
	"github.com/robbiebyrd/indri/internal/transport/graphql/resolvers"
)

const path = "/graphql"

// Transport is the GraphQL transport. It owns the gqlgen server and the set of
// active subscription push connections.
type Transport struct {
	srv *handler.Server

	mu     sync.RWMutex
	sinks  map[transport.Conn]struct{}
	peer   transport.Transport // the aggregate transport, for cross-transport disconnect
	closed bool
}

// New builds the GraphQL transport. sessions resolves bearer tokens to sessions.
func New(sessions resolvers.SessionLookup) *Transport {
	t := &Transport{sinks: make(map[transport.Conn]struct{})}

	resolver := &resolvers.Resolver{Sessions: sessions, Sinks: t}

	srv := handler.New(generated.NewExecutableSchema(generated.Config{Resolvers: resolver}))
	srv.AddTransport(gqlhttp.Options{})
	srv.AddTransport(gqlhttp.GET{})
	srv.AddTransport(gqlhttp.POST{})
	srv.AddTransport(gqlhttp.Websocket{
		KeepAlivePingInterval: 10 * time.Second,
		InitFunc: func(ctx context.Context, initPayload gqlhttp.InitPayload) (context.Context, *gqlhttp.InitPayload, error) {
			// Authenticate the subscription from the connection_init payload's
			// Authorization value (the session bearer token).
			return resolvers.WithToken(ctx, initPayload.Authorization()), &initPayload, nil
		},
	})
	srv.Use(extension.Introspection{})

	t.srv = srv

	return t
}

// SetPeer wires the aggregate transport so Disconnect can reach connections on
// other transports (e.g. a kicked player connected over WebSocket).
func (t *Transport) SetPeer(p transport.Transport) { t.peer = p }

// AddSink registers a subscription's push connection.
func (t *Transport) AddSink(c transport.Conn) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.sinks[c] = struct{}{}
}

// RemoveSink deregisters a subscription's push connection.
func (t *Transport) RemoveSink(c transport.Conn) {
	t.mu.Lock()
	defer t.mu.Unlock()

	delete(t.sinks, c)
}

// Disconnect closes any connection (on any transport) whose session is in ids.
func (t *Transport) Disconnect(ids []string) {
	target := t.peer
	if target == nil {
		target = t
	}

	conns, err := target.Conns()
	if err != nil {
		return
	}

	for _, c := range conns {
		if value, ok := c.Get("sessionId"); ok {
			if id, ok := value.(string); ok && slices.Contains(ids, id) {
				_ = c.Close()
			}
		}
	}
}

// Handle is a no-op for GraphQL: client actions arrive as mutations (handled by
// resolvers), not as opaque messages.
func (t *Transport) Handle(_ transport.Handlers) {}

func (t *Transport) Register(mux *http.ServeMux) {
	mux.Handle(path, authMiddleware(t.srv))
}

func (t *Transport) Broadcast(msg []byte) error {
	for _, c := range t.snapshot() {
		_ = c.Write(msg)
	}

	return nil
}

func (t *Transport) BroadcastFilter(msg []byte, match func(transport.Conn) bool) error {
	for _, c := range t.snapshot() {
		if match(c) {
			_ = c.Write(msg)
		}
	}

	return nil
}

func (t *Transport) Conns() ([]transport.Conn, error) {
	return t.snapshot(), nil
}

func (t *Transport) snapshot() []transport.Conn {
	t.mu.RLock()
	defer t.mu.RUnlock()

	conns := make([]transport.Conn, 0, len(t.sinks))
	for c := range t.sinks {
		conns = append(conns, c)
	}

	return conns
}

func (t *Transport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.closed = true
	for c := range t.sinks {
		_ = c.Close()
	}

	t.sinks = make(map[transport.Conn]struct{})

	return nil
}

func (t *Transport) IsClosed() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.closed
}

// authMiddleware copies the HTTP Authorization header onto the request context
// so mutation resolvers can resolve the caller's session.
func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "" {
			r = r.WithContext(resolvers.WithToken(r.Context(), auth))
		}

		next.ServeHTTP(w, r)
	})
}
