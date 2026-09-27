package connection

import (
	"fmt"
	"log"

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

// disconnectedNotice is written to a connection the server is closing.
const disconnectedNotice = `{"disconnected": true}`

// Close tells the client it is being disconnected, then closes c. It does
// nothing to a connection that is already closed.
func Close(c transport.Conn) {
	if c.IsClosed() {
		return
	}

	if err := c.Write([]byte(disconnectedNotice)); err != nil {
		log.Printf("error writing disconnected: %v", err)
	}

	if err := c.Close(); err != nil {
		log.Printf("error closing connection: %v", err)
	}
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
