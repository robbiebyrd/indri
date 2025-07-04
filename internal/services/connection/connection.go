package connection

import (
	"github.com/olahol/melody"
	melodyClient "github.com/robbiebyrd/indri/internal/clients/melody"
)

type Service struct {
	m *melody.Melody
}

var connectionService *Service

// NewService creates a new repository for accessing user data.
func NewService() *Service {
	if connectionService != nil {
		return connectionService
	}

	m, _ := melodyClient.New()

	connectionService = &Service{m}

	return connectionService
}
