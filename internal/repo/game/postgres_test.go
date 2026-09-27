package game

import (
	"context"
	"errors"
	"os"
	"testing"

	repoErrors "github.com/robbiebyrd/indri/internal/repo"
	"github.com/robbiebyrd/indri/internal/services/events"
	"github.com/robbiebyrd/indri/internal/services/lock"
)

func postgresURI(t *testing.T) string {
	t.Helper()
	uri := os.Getenv("INDRI_TEST_POSTGRES_URI")
	if uri == "" {
		t.Skip("INDRI_TEST_POSTGRES_URI not set; skipping Postgres integration test")
	}
	return uri
}

func newPostgresFixture(t *testing.T) *PostgresStore {
	t.Helper()
	store, err := NewPostgresStore(
		context.Background(),
		postgresURI(t),
		lock.NewInProcess(),
		events.NewInProcess(),
	)
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}
	t.Cleanup(func() { _, _ = store.db.Exec("TRUNCATE TABLE games") })
	return store
}

func TestNewPostgresStore_ReturnsUsableStore(t *testing.T) {
	store := newPostgresFixture(t)
	if store == nil {
		t.Fatal("NewPostgresStore returned nil without error")
	}
}

func TestPostgresStore_NewGame_AssignsIDAndCode(t *testing.T) {
	store := newPostgresFixture(t)
	g, err := store.New("ABCD", makeScript(), false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if g.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if g.Code != "ABCD" || g.Version != 1 {
		t.Errorf("Code=%q Version=%d; want ABCD, 1", g.Code, g.Version)
	}
}

func TestPostgresStore_NewGame_DuplicateCode_ReturnsErrDuplicate(t *testing.T) {
	store := newPostgresFixture(t)
	if _, err := store.New("ABCD", makeScript(), false); err != nil {
		t.Fatalf("first New: %v", err)
	}
	_, err := store.New("ABCD", makeScript(), false)
	if !errors.Is(err, repoErrors.ErrDuplicate) {
		t.Fatalf("expected ErrDuplicate, got %v", err)
	}
}

func TestPostgresStore_Get_UnknownID_ReturnsErrNotFound(t *testing.T) {
	store := newPostgresFixture(t)
	_, err := store.Get("no-such-id")
	if !errors.Is(err, repoErrors.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestPostgresStore_FindByCode(t *testing.T) {
	store := newPostgresFixture(t)
	created, _ := store.New("WXYZ", makeScript(), false)
	got, err := store.FindByCode("WXYZ")
	if err != nil {
		t.Fatalf("FindByCode: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("ID: want %q, got %q", created.ID, got.ID)
	}
}

func TestPostgresStore_Exists(t *testing.T) {
	store := newPostgresFixture(t)
	created, _ := store.New("ABCD", makeScript(), false)
	exists, _ := store.Exists(created.ID)
	if !exists {
		t.Errorf("expected exists=true for created id")
	}
	exists, _ = store.Exists("nope")
	if exists {
		t.Errorf("expected exists=false for unknown id")
	}
}

func TestPostgresStore_FindOpen_ExcludesPrivate(t *testing.T) {
	store := newPostgresFixture(t)
	_, _ = store.New("PUB1", makeScript(), false)
	_, _ = store.New("PRV1", makeScript(), true)
	games, err := store.FindOpen(10)
	if err != nil {
		t.Fatalf("FindOpen: %v", err)
	}
	if len(games) != 1 || games[0].Code != "PUB1" {
		t.Fatalf("want [PUB1], got %v", games)
	}
}
