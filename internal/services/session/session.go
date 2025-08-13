package session

import (
	"fmt"

	"github.com/robbiebyrd/indri/internal/models"
	sessionRepo "github.com/robbiebyrd/indri/internal/repo/session"
)

type Service struct {
	sessionRepo *sessionRepo.Store
}

// NewService creates a new repository for accessing user data.
func NewService(sr *sessionRepo.Store) *Service {
	return &Service{
		sessionRepo: sr,
	}
}

// Sanitize removes private items.
func (us *Service) Sanitize(session *models.Session) *models.Session {
	return session
}

func (us *Service) Get(id string) (*models.Session, error) {
	if id == "" {
		return nil, fmt.Errorf("id is empty")
	}

	thisSession, err := us.sessionRepo.Get(id)
	if err != nil {
		return nil, err
	}

	return thisSession, nil
}

func (us *Service) GetGameIDAndTeamID(id string) (*string, *string, error) {
	thisSession, err := us.Get(id)
	if err != nil {
		return nil, nil, err
	}

	return thisSession.GameID, thisSession.TeamID, nil
}

func (us *Service) GetByUserID(userId string) (*models.Session, error) {
	if userId == "" {
		return nil, fmt.Errorf("userId is empty")
	}

	return us.sessionRepo.FindFirst("userId", userId)
}

func (us *Service) Find(key, value *string) ([]*models.Session, error) {
	if key == nil || value == nil {
		return nil, fmt.Errorf("key and value required to find a session, got %v and %v respectively", key, value)
	}

	return us.sessionRepo.Find(*key, *value)
}

func (us *Service) FindID(key, value string) (*string, error) {
	s, err := us.sessionRepo.FindFirst(key, value)
	if err != nil {
		return nil, err
	}

	id := s.ID.Hex()

	return &id, nil
}

func (us *Service) Exists(id *string) bool {
	if id == nil {
		return false
	}

	exists, err := us.sessionRepo.Exists(*id)
	if err != nil {
		return false
	}

	return exists
}

// Update saves session data to the repository.
func (us *Service) Update(sessionId string, user *models.UpdateSession) error {
	err := us.sessionRepo.Update(sessionId, user)
	if err != nil {
		return err
	}

	return nil
}
