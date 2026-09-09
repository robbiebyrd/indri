package actions

import "github.com/robbiebyrd/indri/internal/models"

// Request is the connection-independent input to an action handler. The
// transport resolves the authenticated session (from a socket key, a bearer
// token, etc.) before dispatch; handlers never touch the connection.
type Request struct {
	// Session is the authenticated session, or nil if the caller is not yet
	// authenticated (register/login/reconnect).
	Session *models.Session
	// Payload is the decoded message/mutation arguments.
	Payload map[string]interface{}
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
