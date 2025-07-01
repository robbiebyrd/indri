package user

import (
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"indri/internal/models"
	ur "indri/internal/repo/user"
)

type Service struct {
	userRepo *ur.Repo
}

var userService *Service

// NewService creates a new repository for accessing user data.
func NewService() *Service {
	if userService == nil {
		userService = &Service{
			userRepo: ur.NewRepo(),
		}
	}

	return userService
}

// Sanitize removes private items.
func (gs *Service) Sanitize(user *models.User) *models.User {
	user.Password = nil
	return user
}

// Get retrieves user data for a specific user ID, and returns an error if not found.
func (gs *Service) Get(id *string) (*models.User, error) {
	if id == nil {
		return nil, fmt.Errorf("id is  nil")
	}

	return gs.userRepo.Get(*id)
}

func (gs *Service) checkPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func (gs *Service) hashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost) // or a higher cost like 14
	return string(bytes), err
}

func (gs *Service) Authenticate(email *string, password *string) (*models.User, error) {
	if password == nil {
		return nil, fmt.Errorf("password cannot be empty")
	}

	storedUser, err := gs.Find(email)
	if err != nil {
		return nil, err
	} else if storedUser.Password == nil {
		return nil, fmt.Errorf("user has not password set")
	}

	if !gs.checkPasswordHash(*password, *storedUser.Password) {
		return nil, fmt.Errorf("password does not match")
	}

	return storedUser, nil
}

// Find retrieves user data for a specific user via email address, and returns an error if not found.
func (gs *Service) Find(email *string) (*models.User, error) {
	if email == nil {
		return nil, fmt.Errorf("email address is required")
	}

	return gs.userRepo.FindFirst("email", *email)
}

// Exists checks to see if a user with the given ID already exists.
func (gs *Service) Exists(id *string) bool {
	if id == nil {
		return false
	}

	exists, err := gs.userRepo.Exists(*id)
	if err != nil {
		return false
	}

	return exists
}

// Update saves user data to the repository.
func (gs *Service) Update(user *models.User) error {
	if user.Email == nil || *user.Email == "" {
		return fmt.Errorf("email address is required")
	}

	if user.Password == nil || *user.Password == "" {
		return fmt.Errorf("password is required")
	}

	hash, err := gs.hashPassword(*user.Password)
	if err != nil {
		return err
	}

	user.Password = &hash

	err = gs.userRepo.Update(user)
	if err != nil {
		return err
	}

	return nil
}

// New creates a new user.
func (gs *Service) New(user *models.User) (*models.User, error) {
	if user.Email == nil || *user.Email == "" {
		return nil, fmt.Errorf("email address is required")
	}

	if user.Password == nil || *user.Password == "" {
		return nil, fmt.Errorf("password is required")
	}

	hash, err := gs.hashPassword(*user.Password)
	if err != nil {
		return nil, err
	}

	user.Password = &hash

	newUser, err := gs.userRepo.New(*user)
	if err != nil {
		return nil, err
	}

	return newUser, nil

}
