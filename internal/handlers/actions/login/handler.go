package login

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
	handlerUtils "github.com/robbiebyrd/indri/internal/handlers/utils"
	"github.com/robbiebyrd/indri/internal/injector"
	"github.com/robbiebyrd/indri/internal/models"
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

	session = h.dropLostSeat(session)

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

// dropLostSeat takes a session that outlived its seat out of its game, so the
// new connection does not receive that game's broadcasts, and returns the
// session as it now stands. Logging in does not depend on the game, so one
// that cannot be read is logged rather than failing the login.
func (h *Handler) dropLostSeat(session *models.Session) *models.Session {
	if session.GameID == nil || *session.GameID == "" {
		return session
	}

	g, err := h.i.GameService.Get(*session.GameID)
	if err != nil {
		log.Printf("login: could not check the seat of session %v in game %v: %v\n", session.ID, *session.GameID, err)

		return session
	}

	held, err := handlerUtils.DropLostSeat(h.i.SessionService, session, g)
	if err != nil {
		log.Printf("login: could not take session %v out of game %v: %v\n", session.ID, g.ID, err)
	}

	if held || err != nil {
		return session
	}

	if fresh, err := h.i.SessionService.Get(session.ID); err == nil {
		return fresh
	}

	return session
}
