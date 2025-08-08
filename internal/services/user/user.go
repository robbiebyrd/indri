package user

import (
	"fmt"

	"github.com/robbiebyrd/indri/internal/models"
	userRepo "github.com/robbiebyrd/indri/internal/repo/user"
	"github.com/robbiebyrd/indri/internal/services/utils"
)

type Service struct {
	userRepo *userRepo.Store
}

// NewService creates a new repository for accessing user data.
func NewService(ur *userRepo.Store) (*Service, error) {
	if ur == nil {
		return nil, fmt.Errorf("userRepo is required")
	}
	return &Service{
		userRepo: ur,
	}, nil
}

// Sanitize removes private items.
func (us *Service) Sanitize(user *models.User) *models.User {
	user.Password = nil
	return user
}

// Get retrieves user data for a specific user ID and returns an error if not found.
func (us *Service) Get(id string) (*models.User, error) {
	if id == "" {
		return nil, fmt.Errorf("id is empty")
	}

	return us.userRepo.Get(id)
}

// Find retrieves user data for a specific user via email address and returns an error if not found.
func (us *Service) Find(email *string) (*models.User, error) {
	if email == nil {
		return nil, fmt.Errorf("email address is required")
	}

	return us.userRepo.FindFirst("email", *email)
}

// Exists checks to see if a user with the given ID already exists.
func (us *Service) Exists(id *string) bool {
	if id == nil {
		return false
	}

	exists, err := us.userRepo.Exists(*id)
	if err != nil {
		return false
	}

	return exists
}

// Update saves user data to the repository.
func (us *Service) Update(user *models.UpdateUser) error {
	if user.Password != nil {
		hash, err := utils.HashPassword(*user.Password)
		if err != nil {
			return err
		}

		user.Password = &hash
	}

	err := us.userRepo.Update(user)
	if err != nil {
		return err
	}

	return nil
}

// New creates a new user.
func (us *Service) New(user models.CreateUser) (*models.User, error) {
	if user.Email == "" {
		return nil, fmt.Errorf("email address is required")
	}

	if user.Password == nil || *user.Password == "" {
		return nil, fmt.Errorf("password is required")
	}

	hash, err := utils.HashPassword(*user.Password)
	if err != nil {
		return nil, err
	}

	user.Password = &hash

	newUser, err := us.userRepo.New(user)
	if err != nil {
		return nil, err
	}

	return newUser, nil
}
