package user

import "github.com/robbiebyrd/indri/internal/models"

// Storer is the contract a user store must satisfy. The assertion below keeps
// it in step with *Store: change one without the other and the build fails.
var _ Storer = (*Store)(nil)

type Storer interface {
	New(user models.CreateUser) (*models.User, error)
	Find(key string, value string) ([]*models.User, error)
	FindFirst(key string, value string) (*models.User, error)
	Get(id string) (*models.User, error)
	Exists(id string) (bool, error)
	Update(user *models.UpdateUser) error
}
