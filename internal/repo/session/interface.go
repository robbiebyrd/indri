package session

import "github.com/robbiebyrd/indri/internal/models"

// Storer is the contract a session store must satisfy. The assertions below
// keep it in step with every backend: change one without the others and the
// build fails.
var (
	_ Storer = (*Store)(nil)
	_ Storer = (*MemoryStore)(nil)
	_ Storer = (*SQLStore)(nil)
)

type Storer interface {
	New(createSession models.CreateSession) (*models.Session, error)
	Find(key string, value string) ([]*models.Session, error)
	FindFirst(key string, value string) (*models.Session, error)
	Get(id string) (*models.Session, error)
	GetByToken(token string) (*models.Session, error)
	Exists(id string) (bool, error)
	Update(sessionId string, session *models.UpdateSession) error
	Delete(id string) error
}
