package connection

import (
	"errors"
	"fmt"

	"github.com/olahol/melody"

	"github.com/robbiebyrd/indri/internal/models"
)

type Service struct {
	s *melody.Session
	m *melody.Melody
}

// NewService creates a new repository for accessing game data.
func NewService(s *melody.Session, m *melody.Melody) *Service {
	return &Service{s, m}
}

// Write accepts a string and writes bytes to the websocket session.
func (ss *Service) Write(data []byte) error {
	return ss.s.Write(data)
}

func (ss *Service) WriteError(error models.WSError) error {
	return ss.s.Write(error.BytesError())
}

// GetKeyAsString gets a session key and returns its value as a string.
func (ss *Service) GetKeyAsString(key string) (*string, error) {
	keyObject, err := ss.GetKey(key)
	if err != nil {
		return nil, err
	}

	keyValue, ok := keyObject.(string)
	if !ok {
		return nil, fmt.Errorf("%v value is not a string", key)
	}

	return &keyValue, nil
}

// GetKey gets a session key and returns its value as a string.
func (ss *Service) GetKey(key string) (any, error) {
	keyObject, ok := ss.s.Get(key)
	if !ok {
		return nil, fmt.Errorf("no %v in session", key)
	}

	return keyObject, nil
}

// SetKey sets a session key.
func (ss *Service) SetKey(key string, data string) {
	ss.s.Set(key, data)
}

func (ss *Service) UnsetKey(key string) {
	ss.s.UnSet(key)
}

func (ss *Service) Get(sessionId *string) (*melody.Session, error) {
	if sessionId != nil {
		return ss.getConnectionForPlayer(*sessionId)
	}

	return nil, errors.New("invalid sessionId")
}

func (ss *Service) getConnectionForPlayer(
	sessionId string,
) (*melody.Session, error) {
	allConnections, err := ss.m.Sessions()
	if err != nil {
		return nil, err
	}

	for _, thisConnection := range allConnections {
		checkSessionId, err := ss.GetKeyAsString("sessionId")
		if err == nil && *checkSessionId == sessionId {
			return thisConnection, nil
		}
	}

	return nil, fmt.Errorf("no sessions were found for sessionId %v", sessionId)
}
