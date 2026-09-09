// Package transport abstracts the client wire protocol behind a small
// interface so it can be swapped without touching handlers, services, or
// routing. WebSocket is the first implementation (see transport/ws); adding
// REST+SSE, WebRTC data channels, WebTransport, or GraphQL subscriptions means
// providing another Transport, not changing anything above this package.
package transport

import "net/http"

// Conn is a single client connection, independent of the wire protocol. It
// carries per-connection state (used to attach a session) and writes messages
// back to that one client.
type Conn interface {
	Get(key string) (value any, exists bool)
	Set(key string, value any)
	UnSet(key string)
	Write(msg []byte) error
	Close() error
	IsClosed() bool
}

// Handlers are the lifecycle callbacks a Transport invokes as connections come
// and go and as messages arrive. Any callback may be nil.
type Handlers struct {
	Connect    func(Conn)
	Disconnect func(Conn)
	Message    func(Conn, []byte)
	Error      func(Conn, error)
}

// Transport accepts client connections over some protocol and delivers their
// lifecycle and messages to the registered Handlers. It also fans messages out
// to connected clients and mounts its own HTTP route(s).
type Transport interface {
	// Handle registers the lifecycle callbacks. Call before Register.
	Handle(Handlers)

	// Register mounts the transport's HTTP endpoint(s) onto mux (e.g. the
	// WebSocket upgrade route, or SSE stream + message POST routes).
	Register(mux *http.ServeMux)

	// Broadcast writes msg to every open connection.
	Broadcast(msg []byte) error

	// BroadcastFilter writes msg to every open connection for which match
	// returns true.
	BroadcastFilter(msg []byte, match func(Conn) bool) error

	// Conns returns the currently open connections (used to locate a specific
	// client, e.g. to force-disconnect it).
	Conns() ([]Conn, error)

	// Close stops accepting connections and closes existing ones.
	Close() error

	// IsClosed reports whether the transport has been closed.
	IsClosed() bool
}
