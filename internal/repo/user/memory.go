package user

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/robbiebyrd/indri/internal/models"
	repoErrors "github.com/robbiebyrd/indri/internal/repo"
	"github.com/robbiebyrd/indri/internal/repo/ids"
)

type MemoryStore struct {
	ctx    context.Context
	mu     sync.RWMutex
	users  map[string]*models.User // id -> user
	emails map[string]string       // email -> id
}

var _ Storer = (*MemoryStore)(nil)

func NewMemoryStore(ctx context.Context) (*MemoryStore, error) {
	return &MemoryStore{
		ctx:    ctx,
		users:  make(map[string]*models.User),
		emails: make(map[string]string),
	}, nil
}

func copyUser(u *models.User) *models.User {
	if u == nil {
		return nil
	}
	out := *u
	return &out
}

func (s *MemoryStore) New(c models.CreateUser) (*models.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.emails[c.Email]; exists {
		return nil, fmt.Errorf("email %q: %w", c.Email, repoErrors.ErrDuplicate)
	}
	now := time.Now()
	u := &models.User{
		ID:          ids.New(),
		Email:       c.Email,
		Name:        c.Name,
		DisplayName: c.DisplayName,
		Password:    c.Password,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.users[u.ID] = u
	s.emails[c.Email] = u.ID
	return copyUser(u), nil
}

func (s *MemoryStore) Get(id string) (*models.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.users[id]
	if !ok {
		return nil, fmt.Errorf("id %q: %w", id, repoErrors.ErrNotFound)
	}
	return copyUser(u), nil
}

func (s *MemoryStore) Exists(id string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.users[id]
	return ok, nil
}

func (s *MemoryStore) Find(key string, value string) ([]*models.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*models.User
	for _, u := range s.users {
		if matchUserField(u, key, value) {
			out = append(out, copyUser(u))
		}
	}
	return out, nil
}

func (s *MemoryStore) FindFirst(key string, value string) (*models.User, error) {
	list, err := s.Find(key, value)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("no user with %s=%q: %w", key, value, repoErrors.ErrNotFound)
	}
	return list[0], nil
}

func (s *MemoryStore) Update(u *models.UpdateUser) error {
	if u == nil || u.ID == "" {
		return errors.New("UpdateUser.ID is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.users[u.ID]
	if !ok {
		return fmt.Errorf("id %q: %w", u.ID, repoErrors.ErrNotFound)
	}
	if u.Email != "" && u.Email != existing.Email {
		if _, dup := s.emails[u.Email]; dup {
			return fmt.Errorf("email %q: %w", u.Email, repoErrors.ErrDuplicate)
		}
		delete(s.emails, existing.Email)
		existing.Email = u.Email
		s.emails[u.Email] = existing.ID
	}
	if u.Name != "" {
		existing.Name = u.Name
	}
	if u.DisplayName != nil {
		existing.DisplayName = u.DisplayName
	}
	if u.Password != nil {
		existing.Password = u.Password
	}
	existing.UpdatedAt = time.Now()
	return nil
}

func matchUserField(u *models.User, key, value string) bool {
	switch key {
	case "email":
		return u.Email == value
	case "name":
		return u.Name == value
	default:
		return false
	}
}
