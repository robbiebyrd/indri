package register

import (
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
	ss := connection.NewService(s, h.i.MelodyClient)
	authExistsErrorMessage := []byte(`{"registered": false, "error": "user already logged in"}`)

	_, err := ss.GetKeyAsString("userId")
	if err == nil {
		_ = ss.Write(authExistsErrorMessage)
		return nil
	}

	msg, err := remarshal(decodedMsg)
	if err != nil {
		return err
	}

	createdUser, err := h.i.UserService.New(*msg)
	if err != nil {
		return err
	}

	err = ss.Write([]byte(fmt.Sprintf(`{"registered": true, "userId": "%s"}`, createdUser.ID.Hex())))
	if err != nil {
		return err
	}

	return nil
}

func remarshal(decodedMsg map[string]interface{}) (*models.CreateUser, error) {
	jsonStr, err := json.Marshal(decodedMsg)
	if err != nil {
		return nil, err
	}

	var msg models.CreateUser

	if err := json.Unmarshal(jsonStr, &msg); err != nil {
		return nil, err
	}

	return &msg, nil
}
