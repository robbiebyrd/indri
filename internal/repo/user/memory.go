package user

import (
	"fmt"
	"sync"

	"github.com/robbiebyrd/indri/internal/models"
	repoErrors "github.com/robbiebyrd/indri/internal/repo"
)

// MemoryStore is a user store held in process memory: for tests, and for a
// server that needs no persistence. It hands out copies, so a caller editing a
// user it read changes nothing until it calls Update, as with a database.
type MemoryStore struct {
	mu      sync.RWMutex
	byID    map[string]*models.User
	byEmail map[string]string // email -> id; the one-account-per-email index
}

// NewMemoryStore builds an empty in-memory user store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		byID:    map[string]*models.User{},
		byEmail: map[string]string{},
	}
}

func copyUser(u *models.User) *models.User {
	out := *u

	return &out
}

func (s *MemoryStore) New(c models.CreateUser) (*models.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, taken := s.byEmail[c.Email]; taken {
		return nil, fmt.Errorf("a user with email %q: %w", c.Email, repoErrors.ErrDuplicate)
	}

	u := newUser(c)
	s.byID[u.ID] = u
	s.byEmail[u.Email] = u.ID

	return copyUser(u), nil
}

func (s *MemoryStore) Get(id string) (*models.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	u, ok := s.byID[id]
	if !ok {
		return nil, fmt.Errorf("user %q: %w", id, repoErrors.ErrNotFound)
	}

	return copyUser(u), nil
}

func (s *MemoryStore) Exists(id string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, ok := s.byID[id]

	return ok, nil
}

func (s *MemoryStore) Find(key string, value string) ([]*models.User, error) {
	if err := requireLookup(key); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var found []*models.User

	for _, u := range s.byID {
		if matches(u, key, value) {
			found = append(found, copyUser(u))
		}
	}

	return found, nil
}

func (s *MemoryStore) FindFirst(key string, value string) (*models.User, error) {
	found, err := s.Find(key, value)
	if err != nil {
		return nil, err
	}

	return first(found, key, value)
}

func (s *MemoryStore) Update(update *models.UpdateUser) error {
	if err := requireID(update); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	u, ok := s.byID[update.ID]
	if !ok {
		return fmt.Errorf("user %q: %w", update.ID, repoErrors.ErrNotFound)
	}

	if update.Email != "" && update.Email != u.Email {
		if _, taken := s.byEmail[update.Email]; taken {
			return fmt.Errorf("a user with email %q: %w", update.Email, repoErrors.ErrDuplicate)
		}

		delete(s.byEmail, u.Email)
		s.byEmail[update.Email] = u.ID
	}

	applyUpdate(u, update)

	return nil
}
