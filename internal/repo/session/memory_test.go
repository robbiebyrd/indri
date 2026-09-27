package session

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

func TestSessionMemoryStore_New_AssignsIDAndKeepsCallerToken(t *testing.T) {
	s := newMemoryFixture(t)
	sess, err := s.New(models.CreateSession{Token: "tok-1", UserID: "u-1"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if sess.ID == "" || sess.Token != "tok-1" {
		t.Errorf("bad session: %+v", sess)
	}
}

func TestSessionMemoryStore_New_DuplicateUserID_ReturnsExisting(t *testing.T) {
	s := newMemoryFixture(t)
	first, _ := s.New(models.CreateSession{Token: "t-1", UserID: "u-1"})
	again, err := s.New(models.CreateSession{Token: "t-2", UserID: "u-1"})
	if err != nil {
		t.Fatalf("New (dup user): %v", err)
	}
	if again.ID != first.ID {
		t.Errorf("expected same session ID; got %q vs %q", again.ID, first.ID)
	}
}

func TestSessionMemoryStore_GetByToken(t *testing.T) {
	s := newMemoryFixture(t)
	first, _ := s.New(models.CreateSession{Token: "t-1", UserID: "u-1"})
	got, err := s.GetByToken("t-1")
	if err != nil || got.ID != first.ID {
		t.Fatalf("GetByToken: got=%v err=%v", got, err)
	}
}

func TestSessionMemoryStore_Delete(t *testing.T) {
	s := newMemoryFixture(t)
	sess, _ := s.New(models.CreateSession{Token: "t-1", UserID: "u-1"})
	if err := s.Delete(sess.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := s.Get(sess.ID)
	if !errors.Is(err, repoErrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound after Delete, got %v", err)
	}
}
