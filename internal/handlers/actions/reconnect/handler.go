package reconnect

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
	token, ok := decodedMsg["sessionId"].(string)
	if !ok || token == "" {
		return fmt.Errorf("sessionId not a string or empty string")
	}

	ss := connection.NewService(s, h.i.MelodyClient)

	// Refuse to resume onto a connection that is already authenticated; the
	// client must log out first so the previous session's presence is cleaned up.
	if _, err := ss.GetKeyAsString("sessionId"); err == nil {
		return fmt.Errorf("connection is already authenticated; log out before reconnecting")
	}

	session, err := h.i.SessionService.GetByToken(token)
	if err != nil {
		return fmt.Errorf("could not resume session: %w", err)
	}

	if session.UserID == nil || *session.UserID == "" {
		return fmt.Errorf("session user id not a string or empty string")
	}

	// Adopt the resumed session on this connection so subsequent authenticated
	// actions and broadcasts target it. The broadcast key is the non-secret
	// session ObjectID, never the token.
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
		[]byte(`{"authenticated": true, "sessionId": "` + token + `", "user": `),
		jsonUserBytes,
		[]byte(`}`),
	}, []byte(""))

	err = ss.Write(authSuccessMessage)
	if err != nil {
		return err
	}

	if session.GameID != nil && *session.GameID != "" {
		g, err := h.i.GameService.Get(*session.GameID)
		if err != nil {
			return err
		}

		jsonGameBytes, err := json.Marshal(h.i.GameService.Sanitize(g))
		if err != nil {
			return err
		}

		err = ss.Write(jsonGameBytes)
		if err != nil {
			return err
		}
	}

	return nil
}
