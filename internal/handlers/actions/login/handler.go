package login

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/olahol/melody"

	"github.com/robbiebyrd/indri/internal/injector"
	"github.com/robbiebyrd/indri/internal/models"
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

	var user *models.User

	currentUserId, err := ss.GetKeyAsString("userId")
	if err != nil {
		session, err := h.i.AuthService.Authenticate(&emailAddress, &password)
		if err != nil {
			return err
		}

		ss.SetKey("sessionId", session.ID.Hex())
		currentUserId = session.UserID
	}

	user, err = h.i.UserService.Get(*currentUserId)
	if err != nil {
		return err
	}

	sessionId, err := ss.GetKeyAsString("sessionId")
	if err != nil {
		return err
	}

	jsonUserBytes, err := json.Marshal(h.i.UserService.Sanitize(user))
	if err != nil {
		return err
	}

	authSuccessMessage := bytes.Join([][]byte{
		[]byte(`{"authenticated": true, "sessionId": "` + *sessionId + `", "user": `),
		jsonUserBytes,
		[]byte(`}`),
	}, []byte(""))

	err = ss.Write(authSuccessMessage)
	if err != nil {
		return err
	}

	return nil
}
