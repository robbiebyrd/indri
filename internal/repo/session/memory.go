package session

import (
	"fmt"
	"sync"

	"github.com/robbiebyrd/indri/internal/models"
	repoErrors "github.com/robbiebyrd/indri/internal/repo"
)

// MemoryStore is a session store held in process memory: for tests, and for a
// server that needs no persistence. It hands out copies, so a caller editing a
// session it read changes nothing until it calls Update, as with a database.
// An expired session is absent to every read and is purged when its user next
// logs in.
type MemoryStore struct {
	mu       sync.RWMutex
	byID     map[string]*models.Session
	byToken  map[string]string // token -> id
	byUserID map[string]string // user id -> id; the one-session-per-user index
}

// NewMemoryStore builds an empty in-memory session store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		byID:     map[string]*models.Session{},
		byToken:  map[string]string{},
		byUserID: map[string]string{},
	}
}

func copySession(s *models.Session) *models.Session {
	out := *s

	return &out
}

// live returns the session with this id if it exists and has not expired.
// Callers hold s.mu.
func (s *MemoryStore) live(id string) (*models.Session, bool) {
	sess, ok := s.byID[id]

	return sess, ok && alive(sess)
}

// remove drops a session from every index. Callers hold s.mu for writing.
func (s *MemoryStore) remove(sess *models.Session) {
	delete(s.byID, sess.ID)

	if sess.Token != "" {
		delete(s.byToken, sess.Token)
	}

	if sess.UserID != nil {
		delete(s.byUserID, *sess.UserID)
	}
}

func (s *MemoryStore) New(c models.CreateSession) (*models.Session, error) {
	if c.UserID == "" {
		return nil, errNoUser
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Expired sessions are purged here, where the store is written anyway, so a
	// long-running server does not keep every session it ever issued.
	for _, sess := range s.byID {
		if !alive(sess) {
			s.remove(sess)
		}
	}

	if id, ok := s.byUserID[c.UserID]; ok {
		return copySession(s.byID[id]), nil
	}

	sess := newSession(c)
	s.byID[sess.ID] = sess
	s.byUserID[c.UserID] = sess.ID

	if sess.Token != "" {
		s.byToken[sess.Token] = sess.ID
	}

	return copySession(sess), nil
}

func (s *MemoryStore) Get(id string) (*models.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sess, ok := s.live(id)
	if !ok {
		return nil, fmt.Errorf("session %q: %w", id, repoErrors.ErrNotFound)
	}

	return copySession(sess), nil
}

func (s *MemoryStore) GetByToken(token string) (*models.Session, error) {
	if err := requireToken(token); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	sess, ok := s.live(s.byToken[token])
	if !ok {
		return nil, fmt.Errorf("session for token: %w", repoErrors.ErrNotFound)
	}

	return copySession(sess), nil
}

func (s *MemoryStore) Exists(id string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, ok := s.live(id)

	return ok, nil
}

func (s *MemoryStore) Find(key string, value string) ([]*models.Session, error) {
	if err := requireLookup(key); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var found []*models.Session

	for _, sess := range s.byID {
		if alive(sess) && matches(sess, key, value) {
			found = append(found, copySession(sess))
		}
	}

	return found, nil
}

func (s *MemoryStore) FindFirst(key string, value string) (*models.Session, error) {
	found, err := s.Find(key, value)
	if err != nil {
		return nil, err
	}

	return first(found, key, value)
}

func (s *MemoryStore) Update(id string, u *models.UpdateSession) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, ok := s.live(id)
	if !ok {
		return fmt.Errorf("session %q: %w", id, repoErrors.ErrNotFound)
	}

	// byUserID is the one-session-per-user index, so it follows a change of
	// user and, like a database's unique index, refuses a user who has one.
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

func (s *MemoryStore) LeaveGame(id string, gameId string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sess, ok := s.live(id); ok && sess.GameID != nil && *sess.GameID == gameId {
		leaveGame(sess)
	}

	return nil
}

// Delete removes a session, invalidating its token. Deleting one that is
// already gone is not an error.
func (s *MemoryStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sess, ok := s.byID[id]; ok {
		s.remove(sess)
	}

	return nil
}
