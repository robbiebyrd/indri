package session

import "github.com/robbiebyrd/indri/internal/models"

type ServiceInterface interface {
	Sanitize(session *models.Session) *models.Session
	Get(id string) (*models.Session, error)
	GetByUserID(userId string) (*models.Session, error)
	Find(key, value *string) ([]*models.Session, error)
	FindID(key, value string) (*string, error)
	Exists(id *string) bool
	Update(sessionId string, user *models.UpdateSession) error
	New(session models.CreateSession) (*models.Session, error)
}
