package login

import (
	"fmt"
	"github.com/olahol/melody"
	"github.com/robbiebyrd/indri/internal/services/session"
	"github.com/robbiebyrd/indri/internal/services/user"
)

var us = user.NewService()

// HandleLogin processes a user login request.
func HandleLogin(
	s *melody.Session,
	decodedMsg map[string]interface{},
) (bool, error) {
	ss := session.NewService(s)
	authSuccessMessage := []byte(`{"authenticated": true}`)

	_, err := ss.GetKeyAsString("userId")
	if err == nil {
		err = s.Write(authSuccessMessage)
		return false, err
	}

	emailAddress, ok := decodedMsg["email"].(string)
	if !ok {
		return false, fmt.Errorf("could not decode email address")
	}

	password, ok := decodedMsg["password"].(string)
	if !ok {
		return false, fmt.Errorf("could not decode password")
	}

	auth, err := us.Authenticate(&emailAddress, &password)
	if err != nil {
		return false, err
	}

	ss.SetKey("userId", auth.ID.Hex())

	err = s.Write(authSuccessMessage)
	if err != nil {
		return false, err
	}

	return false, nil
}

// HandleLogout processes a user logout request.
func HandleLogout(
	s *melody.Session,
	_ map[string]interface{},
) (bool, error) {
	return false, nil
}
