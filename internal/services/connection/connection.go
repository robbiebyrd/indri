package connection

import (
	"github.com/olahol/melody"
	melodyClient "github.com/robbiebyrd/indri/internal/clients/melody"
	"github.com/robbiebyrd/indri/internal/services/user"
)

type Service struct {
	m  *melody.Melody
	us *user.Service
}

var connectionService *Service

// NewService creates a new repository for accessing user data.
func NewService() *Service {
	if connectionService != nil {
		return connectionService
	}

	m := melodyClient.New()
	us := user.NewService()

	connectionService = &Service{m, us}

	return connectionService
}
