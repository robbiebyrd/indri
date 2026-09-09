package login

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
	"github.com/robbiebyrd/indri/internal/injector"
)

type Handler struct {
	i *injector.Injector
}

func New(i *injector.Injector) *Handler {
	return &Handler{i}
}

func (h *Handler) Handle(req actions.Request) (actions.Result, error) {
	// Refuse to re-authenticate a caller that already holds a session; rebinding
	// to a different user would strand the first user's presence.
	if req.Session != nil {
		return actions.Result{}, fmt.Errorf("already authenticated; log out before logging in again")
	}

	emailAddress, ok := req.Payload["email"].(string)
	if !ok || emailAddress == "" {
		return actions.Result{}, fmt.Errorf("email address not a string or empty string")
	}

	password, ok := req.Payload["password"].(string)
	if !ok || password == "" {
		return actions.Result{}, fmt.Errorf("password not a string or empty string")
	}

	session, err := h.i.AuthService.Authenticate(&emailAddress, &password)
	if err != nil {
		return actions.Result{}, err
	}

	if session.UserID == nil || *session.UserID == "" {
		return actions.Result{}, fmt.Errorf("authenticated session has no user id")
	}

	user, err := h.i.UserService.Get(*session.UserID)
	if err != nil {
		return actions.Result{}, err
	}

	jsonUserBytes, err := json.Marshal(h.i.UserService.Sanitize(user))
	if err != nil {
		return actions.Result{}, err
	}

	// The client receives only the secret token, which it echoes back on
	// reconnect. Session is returned so the transport binds its channel.
	authSuccessMessage := bytes.Join([][]byte{
		[]byte(`{"authenticated": true, "sessionId": "` + session.Token + `", "user": `),
		jsonUserBytes,
		[]byte(`}`),
	}, []byte(""))

	return actions.Result{Responses: [][]byte{authSuccessMessage}, Session: session}, nil
}
