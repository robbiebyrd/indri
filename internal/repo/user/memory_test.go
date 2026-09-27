package user

import (
	"context"
	"errors"
	"testing"

	"github.com/robbiebyrd/indri/internal/models"
	repoErrors "github.com/robbiebyrd/indri/internal/repo"
)

func newMemoryFixture(t *testing.T) *MemoryStore {
	t.Helper()
	s, err := NewMemoryStore(context.Background())
	if err != nil {
		t.Fatalf("NewMemoryStore: %v", err)
	}
	return s
}

func TestUserMemoryStore_New_AssignsID(t *testing.T) {
	s := newMemoryFixture(t)
	u, err := s.New(models.CreateUser{Email: "a@b.c", Name: "Alice"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if u.ID == "" {
		t.Fatal("ID empty")
	}
}

func TestUserMemoryStore_New_DuplicateEmail(t *testing.T) {
	s := newMemoryFixture(t)
	_, _ = s.New(models.CreateUser{Email: "a@b.c", Name: "Alice"})
	_, err := s.New(models.CreateUser{Email: "a@b.c", Name: "Alice2"})
	if !errors.Is(err, repoErrors.ErrDuplicate) {
		t.Fatalf("want ErrDuplicate, got %v", err)
	}
}

func TestUserMemoryStore_Get_UnknownID(t *testing.T) {
	s := newMemoryFixture(t)
	_, err := s.Get("nope")
	if !errors.Is(err, repoErrors.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestUserMemoryStore_FindFirst(t *testing.T) {
	s := newMemoryFixture(t)
	u, _ := s.New(models.CreateUser{Email: "a@b.c", Name: "Alice"})
	got, err := s.FindFirst("email", "a@b.c")
	if err != nil || got.ID != u.ID {
		t.Fatalf("FindFirst: got=%v err=%v", got, err)
	}
}
