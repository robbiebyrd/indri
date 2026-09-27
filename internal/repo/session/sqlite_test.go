package session

import (
	"context"
	"errors"
	"testing"

	"github.com/robbiebyrd/indri/internal/clients/sqlite"
	"github.com/robbiebyrd/indri/internal/models"
	repoErrors "github.com/robbiebyrd/indri/internal/repo"
)

func newSQLiteFixture(t *testing.T) *SQLiteStore {
	t.Helper()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	store, err := NewSQLiteStore(context.Background(), db)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return store
}

func TestSessionSQLiteStore_New_AssignsIDAndKeepsCallerToken(t *testing.T) {
	s := newSQLiteFixture(t)
	sess, err := s.New(models.CreateSession{Token: "tok-1", UserID: "u-1"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if sess.ID == "" || sess.Token != "tok-1" {
		t.Errorf("bad session: %+v", sess)
	}
}

func TestSessionSQLiteStore_New_DuplicateUserID_ReturnsExisting(t *testing.T) {
	s := newSQLiteFixture(t)
	first, _ := s.New(models.CreateSession{Token: "t-1", UserID: "u-1"})
	again, err := s.New(models.CreateSession{Token: "t-2", UserID: "u-1"})
	if err != nil {
		t.Fatalf("New (dup user): %v", err)
	}
	if again.ID != first.ID {
		t.Errorf("expected same session ID; got %q vs %q", again.ID, first.ID)
	}
}

func TestSessionSQLiteStore_GetByToken(t *testing.T) {
	s := newSQLiteFixture(t)
	first, _ := s.New(models.CreateSession{Token: "t-1", UserID: "u-1"})
	got, err := s.GetByToken("t-1")
	if err != nil || got.ID != first.ID {
		t.Fatalf("GetByToken: got=%v err=%v", got, err)
	}
}

func TestSessionSQLiteStore_Delete(t *testing.T) {
	s := newSQLiteFixture(t)
	sess, _ := s.New(models.CreateSession{Token: "t-1", UserID: "u-1"})
	if err := s.Delete(sess.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := s.Get(sess.ID)
	if !errors.Is(err, repoErrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound after Delete, got %v", err)
	}
}
