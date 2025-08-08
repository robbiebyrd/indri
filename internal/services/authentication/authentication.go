package authentication

import (
	"errors"
	"fmt"
	"time"

	"github.com/robbiebyrd/indri/internal/models"
	sessionRepo "github.com/robbiebyrd/indri/internal/repo/session"
	userRepo "github.com/robbiebyrd/indri/internal/repo/user"
	"github.com/robbiebyrd/indri/internal/services/utils"
)

type Service struct {
	userRepo    *userRepo.Store
	sessionRepo *sessionRepo.Store
}

// NewService creates a new repository for accessing user data.
func NewService(userRepo *userRepo.Store, sessionRepo *sessionRepo.Store) (*Service, error) {
	if userRepo == nil {
		return nil, errors.New("userRepo is required")
	}

	if sessionRepo == nil {
		return nil, errors.New("userRepo is required")
	}

	return &Service{
		userRepo:    userRepo,
		sessionRepo: sessionRepo,
	}, nil
}

func (us *Service) Authenticate(email *string, password *string) (*models.Session, error) {
	if password == nil || *password == "" {
		return nil, fmt.Errorf("password cannot be nil or empty")
	} else if email == nil || *email == "" {
		return nil, fmt.Errorf("email cannot be nil or empty")
	}

	storedUser, err := us.userRepo.FindFirst("email", *email)
	if err != nil {
		return nil, fmt.Errorf("user not found when authenticating: %v", err)
	} else if storedUser.Password == nil {
		return nil, fmt.Errorf("user has not password set")
	}

	if !utils.CheckPasswordHash(*password, *storedUser.Password) {
		return nil, fmt.Errorf("password does not match")
	}

	session, err := us.sessionRepo.New(models.CreateSession{
		UserID:    storedUser.ID.Hex(),
		CreatedAt: time.Time{},
	})
	if err != nil {
		return nil, fmt.Errorf("could not create session: %v", err)
	}

	return session, nil
}
