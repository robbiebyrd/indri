package resolvers

import (
	"context"
	"strings"

	"github.com/robbiebyrd/indri/internal/handlers/router"
	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/transport"
	"github.com/robbiebyrd/indri/internal/transport/graphql/model"
)

// SessionLookup resolves a session from its bearer token (the same opaque token
// login/reconnect issue). Satisfied by the session service.
type SessionLookup interface {
	GetByToken(token string) (*models.Session, error)
}

// Sinks lets a subscription register/unregister its push connection, and lets a
// mutation force-disconnect sessions (kick/logout) across all transports.
type Sinks interface {
	AddSink(c transport.Conn)
	RemoveSink(c transport.Conn)
	Disconnect(sessionIDs []string)
}

// Resolver is the gqlgen root resolver. It holds only narrow dependencies so
// the graphql package doesn't pull in the injector.
type Resolver struct {
	Sessions SessionLookup
	Sinks    Sinks
}

type ctxKey int

const tokenCtxKey ctxKey = iota

// WithToken stores the caller's bearer token on the context (set by the HTTP
// Authorization middleware and the websocket connection_init handler).
func WithToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, tokenCtxKey, strings.TrimPrefix(token, "Bearer "))
}

func tokenFromContext(ctx context.Context) string {
	token, _ := ctx.Value(tokenCtxKey).(string)
	return token
}

// sessionFromContext resolves the authenticated session from the context token,
// or nil if there is no valid token.
func (r *Resolver) sessionFromContext(ctx context.Context) *models.Session {
	token := tokenFromContext(ctx)
	if token == "" {
		return nil
	}

	session, err := r.Sessions.GetByToken(token)
	if err != nil {
		return nil
	}

	return session
}

// dispatch runs an action through the shared dispatcher and returns its first
// response document as the mutation result. DisconnectIDs (kick/logout) are
// applied against every transport.
func (r *Resolver) dispatch(ctx context.Context, action string, payload map[string]interface{}) (model.JSON, error) {
	session := r.sessionFromContext(ctx)

	result, err := router.Dispatch(session, action, payload)

	if len(result.DisconnectIDs) > 0 {
		r.Sinks.Disconnect(result.DisconnectIDs)
	}

	var out model.JSON
	if len(result.Responses) > 0 {
		out = model.JSON(result.Responses[0])
	}

	return out, err
}
