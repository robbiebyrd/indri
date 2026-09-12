package actions

import (
	"context"

	"github.com/robbiebyrd/indri/internal/models"
)

// Request is the connection-independent input to an action handler. The
// transport resolves the authenticated session (from a socket key, a bearer
// token, etc.) before dispatch; handlers never touch the connection.
type Request struct {
	// Context is the caller's request context. Handlers must pass it to every
	// blocking call they make (game locks, database reads, writes), so a
	// cancelled or expired request unwinds instead of pinning a goroutine.
	Context context.Context
	// Action is the action that was dispatched. The router runs a handler under
	// the received/<action>/processed lifecycle, so a hook registered on
	// received or processed reads this to learn which action actually fired.
	Action string
	// Session is the authenticated session, or nil if the caller is not yet
	// authenticated (register/login/reconnect).
	Session *models.Session
	// Payload is the decoded message/mutation arguments.
	Payload map[string]interface{}
}

// Ctx returns the request context, substituting context.Background() for a
// Request built without one. Dispatch always sets Context; this only keeps a
// hand-built Request (a test, a caller that bypasses the router) from panicking
// deep inside a lock or driver call.
func (r Request) Ctx() context.Context {
	if r.Context == nil {
		return context.Background()
	}

	return r.Context
}

// Result is what an action handler returns. The transport decides how to
// deliver it (write frames back to a socket, return a mutation payload, ...).
type Result struct {
	// Responses are messages to send back to the caller, in order.
	Responses [][]byte
	// Session is set when this call establishes or refreshes authentication, so
	// the transport can bind its channel to the session.
	Session *models.Session
	// DisconnectIDs are sessionIds whose connections should be force-closed
	// (kick, logout).
	DisconnectIDs []string
}

// MessageHandler processes one action. It is connection-agnostic: input is a
// Request, output is a Result plus an error.
type MessageHandler interface {
	Handle(Request) (Result, error)
}
