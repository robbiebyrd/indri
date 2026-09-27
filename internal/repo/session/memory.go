package session

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/robbiebyrd/indri/internal/models"
	repoErrors "github.com/robbiebyrd/indri/internal/repo"
)

type MemoryStore struct {
	ctx      context.Context
	mu       sync.RWMutex
	byID     map[string]*models.Session
	byToken  map[string]string // token -> id
	byUserID map[string]string // userID -> id (unique)
}

var _ Storer = (*MemoryStore)(nil)

func NewMemoryStore(ctx context.Context) (*MemoryStore, error) {
	return &MemoryStore{
		ctx:      ctx,
		byID:     make(map[string]*models.Session),
		byToken:  make(map[string]string),
		byUserID: make(map[string]string),
	}, nil
}

func copySession(s *models.Session) *models.Session {
	if s == nil {
		return nil
	}
	out := *s
	return &out
}

// New enforces "one session per userId" like MongoStore does: a second call
// for the same UserID returns the existing session.
func (s *MemoryStore) New(c models.CreateSession) (*models.Session, error) {
	if c.UserID == "" {
		return nil, errors.New("session must have a user id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if existingID, ok := s.byUserID[c.UserID]; ok {
		return copySession(s.byID[existingID]), nil
	}
	sess := newSession(c)
	s.byID[sess.ID] = sess
	if sess.Token != "" {
		s.byToken[sess.Token] = sess.ID
	}
	s.byUserID[c.UserID] = sess.ID
	return copySession(sess), nil
}

func (s *MemoryStore) Get(id string) (*models.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.byID[id]
	if !ok {
		return nil, fmt.Errorf("id %q: %w", id, repoErrors.ErrNotFound)
	}
	return copySession(sess), nil
}

func (s *MemoryStore) GetByToken(token string) (*models.Session, error) {
	if token == "" {
		return nil, errors.New("token is empty")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.byToken[token]
	if !ok {
		return nil, fmt.Errorf("token: %w", repoErrors.ErrNotFound)
	}
	return copySession(s.byID[id]), nil
}

func (s *MemoryStore) Exists(id string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.byID[id]
	return ok, nil
}

func (s *MemoryStore) Find(key string, value string) ([]*models.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*models.Session
	for _, sess := range s.byID {
		if matchSessionField(sess, key, value) {
			out = append(out, copySession(sess))
		}
	}
	return out, nil
}

func (s *MemoryStore) FindFirst(key string, value string) (*models.Session, error) {
	list, err := s.Find(key, value)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("%s=%q: %w", key, value, repoErrors.ErrNotFound)
	}
	return list[0], nil
}

func (s *MemoryStore) Update(id string, u *models.UpdateSession) error {
	if u == nil {
		return errors.New("UpdateSession is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.byID[id]
	if !ok {
		return fmt.Errorf("id %q: %w", id, repoErrors.ErrNotFound)
	}
	// byUserID is the one-session-per-user index, so it follows a change
	// of user; like the other stores' unique index, it refuses a user who
	// already has a session.
	if u.UserID != "" && (sess.UserID == nil || *sess.UserID != u.UserID) {
		if other, taken := s.byUserID[u.UserID]; taken && other != id {
			return fmt.Errorf("user %q already has a session: %w", u.UserID, repoErrors.ErrDuplicate)
		}
		if sess.UserID != nil {
			delete(s.byUserID, *sess.UserID)
		}
		s.byUserID[u.UserID] = id
	}
	applyUpdate(sess, u)
	return nil
}

func (s *MemoryStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.byID[id]
	if !ok {
		return nil // idempotent, matches MongoStore
	}
	delete(s.byID, id)
	if sess.Token != "" {
		delete(s.byToken, sess.Token)
	}
	if sess.UserID != nil {
		delete(s.byUserID, *sess.UserID)
	}
	return nil
}

func matchSessionField(sess *models.Session, key, value string) bool {
	switch key {
	case "token":
		return sess.Token == value
	case "userId":
		return sess.UserID != nil && *sess.UserID == value
	case "gameId":
		return sess.GameID != nil && *sess.GameID == value
	case "teamId":
		return sess.TeamID != nil && *sess.TeamID == value
	default:
		return false
	}
}
