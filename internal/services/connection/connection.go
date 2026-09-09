package connection

import (
	"errors"
	"fmt"

	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/transport"
)

type Service struct {
	c transport.Conn
	t transport.Transport
}

// NewService wraps a single connection (and its transport, for cross-connection
// lookups) with typed helpers for reading/writing per-connection session state.
func NewService(c transport.Conn, t transport.Transport) *Service {
	return &Service{c, t}
}

// Write accepts a string and writes bytes to the connection.
func (ss *Service) Write(data []byte) error {
	return ss.c.Write(data)
}

func (ss *Service) WriteError(error models.WSError) error {
	return ss.c.Write(error.BytesError())
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

// GetKey gets a session key and returns its value.
func (ss *Service) GetKey(key string) (any, error) {
	keyObject, ok := ss.c.Get(key)
	if !ok {
		return nil, fmt.Errorf("no %v in session", key)
	}

	return keyObject, nil
}

// SetKey sets a session key.
func (ss *Service) SetKey(key string, data string) {
	ss.c.Set(key, data)
}

func (ss *Service) UnsetKey(key string) {
	ss.c.UnSet(key)
}

// Get returns the connection whose "sessionId" key matches sessionId, if it is
// currently connected.
func (ss *Service) Get(sessionId *string) (transport.Conn, error) {
	if sessionId != nil {
		return ss.getConnectionForPlayer(*sessionId)
	}

	return nil, errors.New("invalid sessionId")
}

func (ss *Service) getConnectionForPlayer(sessionId string) (transport.Conn, error) {
	conns, err := ss.t.Conns()
	if err != nil {
		return nil, err
	}

	for _, c := range conns {
		checkSessionId, err := NewService(c, ss.t).GetKeyAsString("sessionId")
		if err == nil && *checkSessionId == sessionId {
			return c, nil
		}
	}

	return nil, fmt.Errorf("no sessions were found for sessionId %v", sessionId)
}
