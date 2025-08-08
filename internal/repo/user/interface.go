package user

import "github.com/robbiebyrd/indri/internal/models"

type Storer interface {
	New(user models.CreateUser) (*models.User, error)
	Find(key string, value string) ([]*models.User, error)
	FindFirst(key string, value string) (*models.User, error)
	Get(id string) (*models.User, error)
	Exists(id string) (bool, error)
	Update(user *models.UpdateUser) error
}
