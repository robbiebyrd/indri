package session

import "github.com/robbiebyrd/indri/internal/models"

type Storer interface {
	New(createSession models.CreateSession) (*models.Session, error)
	Find(key string, value string) ([]*models.Session, error)
	FindFirst(key string, value string) (*models.Session, error)
	Get(id string) (*models.Session, error)
	Exists(id string) (bool, error)
	Update(sessionId string, session *models.UpdateSession) error
}
