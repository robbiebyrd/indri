package login

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/olahol/melody"

	"github.com/robbiebyrd/indri/internal/injector"
	"github.com/robbiebyrd/indri/internal/services/connection"
)

type Handler struct {
	i *injector.Injector
}

func New(i *injector.Injector) *Handler {
	return &Handler{i}
}

func (h *Handler) Handle(
	s *melody.Session,
	decodedMsg map[string]interface{},
) error {
	emailAddress, ok := decodedMsg["email"].(string)
	if !ok || emailAddress == "" {
		return fmt.Errorf("email address not a string or empty string")
	}

	password, ok := decodedMsg["password"].(string)
	if !ok || password == "" {
		return fmt.Errorf("password not a string or empty string")
	}

	ss := connection.NewService(s, h.i.MelodyClient)

	// Refuse to re-authenticate a connection that already holds a session;
	// rebinding it to a different user would leave the first user's presence
	// (connected: true) stranded. The client must log out first.
	if _, err := ss.GetKeyAsString("sessionId"); err == nil {
		return fmt.Errorf("connection is already authenticated; log out before logging in again")
	}

	session, err := h.i.AuthService.Authenticate(&emailAddress, &password)
	if err != nil {
		return err
	}

	if session.UserID == nil || *session.UserID == "" {
		return fmt.Errorf("authenticated session has no user id")
	}

	// The server-side targeting key uses the non-secret session ObjectID so
	// broadcasts can find this connection. The client only ever receives the
	// secret token, which it echoes back on reconnect.
	ss.SetKey("sessionId", session.ID.Hex())

	user, err := h.i.UserService.Get(*session.UserID)
	if err != nil {
		return err
	}

	jsonUserBytes, err := json.Marshal(h.i.UserService.Sanitize(user))
	if err != nil {
		return err
	}

	authSuccessMessage := bytes.Join([][]byte{
		[]byte(`{"authenticated": true, "sessionId": "` + session.Token + `", "user": `),
		jsonUserBytes,
		[]byte(`}`),
	}, []byte(""))

	err = ss.Write(authSuccessMessage)
	if err != nil {
		return err
	}

	return nil
}
