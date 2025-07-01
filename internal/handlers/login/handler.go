package login

import (
	"fmt"
	"github.com/olahol/melody"
	"indri/internal/entrypoints"
	"indri/internal/models"
	"indri/internal/services/session"
	"indri/internal/services/user"
)

var us = user.NewService()

// HandleLogin processes a user login request.
func HandleLogin(
	s *melody.Session,
	_ *melody.Melody,
	_ *models.Game,
	decodedMsg map[string]interface{},
) error {
	ss := session.NewService(s)
	authSuccessMessage := []byte(`{"authenticated": true}`)

	_, err := ss.GetKeyAsString("userId")
	if err == nil {
		err = s.Write(authSuccessMessage)
		if err != nil {
			return err
		}
		return nil
	}

	emailAddress, ok := decodedMsg["email"].(string)
	if !ok {
		return fmt.Errorf("email address not a string")
	}

	password, ok := decodedMsg["password"].(string)
	if !ok {
		return fmt.Errorf("password not a string")
	}

	auth, err := us.Authenticate(&emailAddress, &password)
	if err != nil {
		return err
	}

	ss.SetKey( "userId", auth.ID.Hex())

	err = s.Write(authSuccessMessage)
	if err != nil {
		return err
	}

	return nil
}

// HandleLogout processes a user logout request.
func HandleLogout(
	s *melody.Session,
	m *melody.Melody,
	g *models.Game,
	_ map[string]interface{},
) error {
	entrypoints.HandleDisconnect(s, m)
	return nil
}
