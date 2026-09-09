package resolvers

// This file will be automatically regenerated based on the schema, any resolver
// implementations will be copied through when generating and any unknown code
// will be moved to the end.

import (
	"context"
	"fmt"

	"github.com/robbiebyrd/indri/internal/transport/graphql/generated"
	"github.com/robbiebyrd/indri/internal/transport/graphql/model"
)

// Register is the resolver for the register field.
func (r *mutationResolver) Register(ctx context.Context, email string, password string, name string) (model.JSON, error) {
	return r.dispatch(ctx, "register", map[string]interface{}{
		"email": email, "password": password, "name": name,
	})
}

// Login is the resolver for the login field.
func (r *mutationResolver) Login(ctx context.Context, email string, password string) (model.JSON, error) {
	return r.dispatch(ctx, "login", map[string]interface{}{
		"email": email, "password": password,
	})
}

// Reconnect is the resolver for the reconnect field.
func (r *mutationResolver) Reconnect(ctx context.Context, token string) (model.JSON, error) {
	// The reconnect action reads the token from the "sessionId" payload field.
	return r.dispatch(ctx, "reconnect", map[string]interface{}{"sessionId": token})
}

// CreateGame is the resolver for the createGame field.
func (r *mutationResolver) CreateGame(ctx context.Context, code string, teamID string, private *bool) (model.JSON, error) {
	isPrivate := false
	if private != nil {
		isPrivate = *private
	}

	return r.dispatch(ctx, "create", map[string]interface{}{
		"code": code, "teamId": teamID, "private": isPrivate,
	})
}

// JoinGame is the resolver for the joinGame field.
func (r *mutationResolver) JoinGame(ctx context.Context, code string, teamID string) (model.JSON, error) {
	return r.dispatch(ctx, "join", map[string]interface{}{
		"code": code, "teamId": teamID,
	})
}

// LeaveGame is the resolver for the leaveGame field.
func (r *mutationResolver) LeaveGame(ctx context.Context) (model.JSON, error) {
	return r.dispatch(ctx, "leave", map[string]interface{}{})
}

// Kick is the resolver for the kick field.
func (r *mutationResolver) Kick(ctx context.Context, code string, userID string) (model.JSON, error) {
	return r.dispatch(ctx, "kick", map[string]interface{}{"code": code, "userId": userID})
}

// Logout is the resolver for the logout field.
func (r *mutationResolver) Logout(ctx context.Context) (model.JSON, error) {
	return r.dispatch(ctx, "logout", map[string]interface{}{})
}

// Inquire is the resolver for the inquire field.
func (r *mutationResolver) Inquire(ctx context.Context, inquiryType string, inquiry *string, code *string) (model.JSON, error) {
	payload := map[string]interface{}{"inquiryType": inquiryType}
	if inquiry != nil {
		payload["inquiry"] = *inquiry
	}
	if code != nil {
		payload["code"] = *code
	}

	return r.dispatch(ctx, "inquire", payload)
}

// Ping is the resolver for the ping field.
func (r *queryResolver) Ping(ctx context.Context) (string, error) {
	return "pong", nil
}

// GameUpdates is the resolver for the gameUpdates field.
func (r *subscriptionResolver) GameUpdates(ctx context.Context, gameID string) (<-chan model.JSON, error) {
	session := r.sessionFromContext(ctx)
	if session == nil {
		return nil, fmt.Errorf("unauthenticated subscription: missing or invalid token")
	}

	// The push conn is keyed by the session ObjectID, so the existing broadcast
	// fan-out reaches it exactly like a WebSocket client in the same game.
	conn := newSubConn(session.ID.Hex())
	r.Sinks.AddSink(conn)

	go func() {
		<-ctx.Done()
		r.Sinks.RemoveSink(conn)
		_ = conn.Close()
	}()

	return conn.events(), nil
}

// Mutation returns generated.MutationResolver implementation.
func (r *Resolver) Mutation() generated.MutationResolver { return &mutationResolver{r} }

// Query returns generated.QueryResolver implementation.
func (r *Resolver) Query() generated.QueryResolver { return &queryResolver{r} }

// Subscription returns generated.SubscriptionResolver implementation.
func (r *Resolver) Subscription() generated.SubscriptionResolver { return &subscriptionResolver{r} }

type (
	mutationResolver     struct{ *Resolver }
	queryResolver        struct{ *Resolver }
	subscriptionResolver struct{ *Resolver }
)
