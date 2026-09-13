// Package graphql implements transport.Transport over GraphQL: typed mutations
// (client -> server) dispatched to the shared action logic, and a gameUpdates
// subscription (server -> client) fed by the existing broadcast fan-out. It is
// mounted at /graphql and runs alongside the WebSocket transport.
package graphql

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	gqlhttp "github.com/99designs/gqlgen/graphql/handler/transport"
	coderws "github.com/coder/websocket"

	"github.com/robbiebyrd/indri/internal/transport"
	"github.com/robbiebyrd/indri/internal/transport/graphql/generated"
	"github.com/robbiebyrd/indri/internal/transport/graphql/resolvers"
)

const path = "/graphql"

// Transport is the GraphQL transport. It owns the gqlgen server; the set of
// active subscription push connections and the fan-out over them come from the
// embedded Registry, shared with the other push-only transports.
type Transport struct {
	*transport.Registry

	srv *handler.Server

	// origins guards the subscription upgrade. The CORS middleware in
	// internal/entrypoints/http covers the HTTP half of this transport, but a
	// WebSocket upgrade is not a CORS request and is decided by the upgrader.
	origins *transport.OriginPolicy
}

// New builds the GraphQL transport. sessions resolves bearer tokens to sessions.
func New(sessions resolvers.SessionLookup) *Transport {
	t := &Transport{
		Registry: &transport.Registry{},
		origins:  transport.OriginPolicyFromEnv(),
	}

	resolver := &resolvers.Resolver{Sessions: sessions, Sinks: t}

	srv := handler.New(generated.NewExecutableSchema(generated.Config{Resolvers: resolver}))
	srv.AddTransport(gqlhttp.Options{})
	srv.AddTransport(gqlhttp.GET{})
	srv.AddTransport(gqlhttp.POST{})
	srv.AddTransport(gqlhttp.Websocket{
		KeepAlivePingInterval: 10 * time.Second,
		Implementation:        originGuard{transport: t},
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

// originGuard applies the shared origin policy to the subscription upgrade. A
// WebSocket handshake is not a CORS request, so the middleware in
// internal/entrypoints/http is not what decides it; without this, gqlgen's
// websocket library falls back to its own same-origin rule and refuses an
// allowlisted browser that every other route accepts.
// It holds the transport rather than the policy so the policy is read at accept
// time. Capturing it here instead would freeze whatever New built, leaving no
// way to substitute one afterwards.
type originGuard struct {
	transport *Transport
}

var _ gqlhttp.WebsocketImplementation = originGuard{}

// Accept refuses a disallowed origin, then hands off to gqlgen's default
// implementation. That implementation's own origin check is disabled because
// this one has already made the decision, from the same allowlist every other
// transport reads.
//
// The error is returned rather than written: gqlgen answers a failed accept
// itself, and writing here as well would be a second, superfluous response.
func (g originGuard) Accept(
	w http.ResponseWriter,
	r *http.Request,
	options gqlhttp.WebsocketAcceptOptions,
) (gqlhttp.WebsocketConn, error) {
	if !g.transport.origins.Allows(r) {
		return nil, fmt.Errorf("origin %q is not allowed", r.Header.Get("Origin"))
	}

	next := gqlhttp.CoderWebsocketImplementation{
		AcceptOptions: coderws.AcceptOptions{InsecureSkipVerify: true},
	}

	return next.Accept(w, r, options)
}

// Handle is a no-op for GraphQL: client actions arrive as mutations (handled by
// resolvers), not as opaque messages.
func (t *Transport) Handle(_ transport.Handlers) {}

// Register mounts the GraphQL endpoint. The body limit is outermost so an
// oversized operation is refused before anything reads or authenticates it; it
// leaves the subscription upgrade, which has no body, alone.
func (t *Transport) Register(mux *http.ServeMux) {
	mux.Handle(path, transport.LimitBody(authMiddleware(t.srv)))
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
