package user

import (
	"fmt"
	"github.com/robbiebyrd/indri/internal/models"
	ur "github.com/robbiebyrd/indri/internal/repo/user"
	"golang.org/x/crypto/bcrypt"
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
func (us *Service) Sanitize(user *models.User) *models.User {
	user.Password = nil
	return user
}

// Get retrieves user data for a specific user ID, and returns an error if not found.
func (us *Service) Get(id *string) (*models.User, error) {
	if id == nil {
		return nil, fmt.Errorf("id is  nil")
	}

	return us.userRepo.Get(*id)
}

func (us *Service) Authenticate(email *string, password *string) (*models.User, error) {
	if password == nil {
		return nil, fmt.Errorf("password cannot be empty")
	}

	storedUser, err := us.Find(email)
	if err != nil {
		return nil, err
	} else if storedUser.Password == nil {
		return nil, fmt.Errorf("user has not password set")
	}

	if !us.checkPasswordHash(*password, *storedUser.Password) {
		return nil, fmt.Errorf("password does not match")
	}

	return storedUser, nil
}

// Find retrieves user data for a specific user via email address, and returns an error if not found.
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
		hash, err := us.hashPassword(*user.Password)
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

	if user.Password == "" {
		return nil, fmt.Errorf("password is required")
	}

	if user.Password == "" {
		return nil, fmt.Errorf("password is required")
	}

	hash, err := us.hashPassword(user.Password)
	if err != nil {
		return nil, err
	}

	user.Password = hash

	newUser, err := us.userRepo.New(user)
	if err != nil {
		return nil, err
	}

	return newUser, nil
}

func (us *Service) checkPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func (us *Service) hashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost) // or a higher cost like 14
	return string(bytes), err
}
