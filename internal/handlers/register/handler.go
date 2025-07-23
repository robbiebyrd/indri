package register

import (
	"encoding/json"
	"fmt"
	"github.com/olahol/melody"
	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/services/session"
	"github.com/robbiebyrd/indri/internal/services/user"
)

var us = user.NewService()

// HandleRegister processes a user login request.
func HandleRegister(
	s *melody.Session,
	decodedMsg map[string]interface{},
) error {
	ss := session.NewService(s)
	authExistsErrorMessage := []byte(`{"registered": false, "error": "user already logged in"}`)

	_, err := ss.GetKeyAsString("userId")
	if err == nil {
		s.Write(authExistsErrorMessage)
		return nil
	}

	msg, err := remarshal(decodedMsg)
	if err != nil {
		return err
	}

	createdUser, err := us.New(*msg)
	if err != nil {
		return err
	}

	err = s.Write([]byte(fmt.Sprintf(`{"registered": true, "userId": "%s"}`, createdUser.ID)))
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
