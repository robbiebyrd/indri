package session

import (
	"context"
	"errors"
	"os"
	"testing"

	postgresClient "github.com/robbiebyrd/indri/internal/clients/postgres"
	"github.com/robbiebyrd/indri/internal/models"
	repoErrors "github.com/robbiebyrd/indri/internal/repo"
)

func postgresSessionURI(t *testing.T) string {
	t.Helper()
	uri := os.Getenv("INDRI_TEST_POSTGRES_URI")
	if uri == "" {
		t.Skip("INDRI_TEST_POSTGRES_URI not set; skipping Postgres integration test")
	}
	return uri
}

func newPostgresSessionFixture(t *testing.T) *PostgresStore {
	t.Helper()
	db, err := postgresClient.Open(context.Background(), postgresSessionURI(t))
	if err != nil {
		t.Fatalf("postgres Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store, err := NewPostgresStore(context.Background(), db)
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec("TRUNCATE TABLE sessions") })
	return store
}

func TestSessionPostgresStore_New_AssignsIDAndKeepsToken(t *testing.T) {
	s := newPostgresSessionFixture(t)
	sess, err := s.New(models.CreateSession{Token: "tok-1", UserID: "u-1"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if sess.ID == "" || sess.Token != "tok-1" {
		t.Errorf("bad session: %+v", sess)
	}
}

func TestSessionPostgresStore_New_DuplicateUserID_ReturnsExisting(t *testing.T) {
	s := newPostgresSessionFixture(t)
	first, _ := s.New(models.CreateSession{Token: "t-1", UserID: "u-1"})
	again, err := s.New(models.CreateSession{Token: "t-2", UserID: "u-1"})
	if err != nil {
		t.Fatalf("New (dup user): %v", err)
	}
	if again.ID != first.ID {
		t.Errorf("expected same session ID; got %q vs %q", again.ID, first.ID)
	}
}

func TestSessionPostgresStore_GetByToken(t *testing.T) {
	s := newPostgresSessionFixture(t)
	first, _ := s.New(models.CreateSession{Token: "t-1", UserID: "u-1"})
	got, err := s.GetByToken("t-1")
	if err != nil || got.ID != first.ID {
		t.Fatalf("GetByToken: got=%v err=%v", got, err)
	}
}

func TestSessionPostgresStore_Delete(t *testing.T) {
	s := newPostgresSessionFixture(t)
	sess, _ := s.New(models.CreateSession{Token: "t-1", UserID: "u-1"})
	if err := s.Delete(sess.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := s.Get(sess.ID)
	if !errors.Is(err, repoErrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound after Delete, got %v", err)
	}
}

// TestSessionPostgresStore_Get_TokenRoundtrips proves models.Session.Token
// (json:"-", so it is dropped from the JSONB blob) survives a New -> Get
// round-trip via the dedicated token column.
func TestSessionPostgresStore_Get_TokenRoundtrips(t *testing.T) {
	s := newPostgresSessionFixture(t)
	created, err := s.New(models.CreateSession{Token: "tok-1", UserID: "u-1"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	got, err := s.Get(created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Token != "tok-1" {
		t.Fatalf("Get: token did not round-trip, got %q", got.Token)
	}
}

// TestSessionPostgresStore_FindFirst_TokenRoundtrips proves the token column
// is also restored on the Find/FindFirst read path, not just Get.
func TestSessionPostgresStore_FindFirst_TokenRoundtrips(t *testing.T) {
	s := newPostgresSessionFixture(t)
	_, err := s.New(models.CreateSession{Token: "tok-1", UserID: "u-1"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	got, err := s.FindFirst("userId", "u-1")
	if err != nil {
		t.Fatalf("FindFirst: %v", err)
	}
	if got.Token != "tok-1" {
		t.Fatalf("FindFirst: token did not round-trip, got %q", got.Token)
	}
}
