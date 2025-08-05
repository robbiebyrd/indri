package connection

import (
	"context"
	"errors"
	"github.com/olahol/melody"
	userRepo "github.com/robbiebyrd/indri/internal/repo/user"
)

type Service struct {
	m  *melody.Melody
	ur *userRepo.Repo
}

// NewService creates a new repository for accessing user data.
func NewService(ctx context.Context, m *melody.Melody, userRepo *userRepo.Repo) (*Service, error) {
	if ctx == nil {
		return nil, errors.New("context was not passed to the connection service")
	}

	if m == nil {
		return nil, errors.New("melody client was not passed to the connection service")
	}

	if userRepo == nil {
		return nil, errors.New("user repo was not passed to the connection service")
	}

	return &Service{m, userRepo}, nil
}
