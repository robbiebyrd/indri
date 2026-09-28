package game

import (
	"context"
	"errors"
	"testing"

	"github.com/robbiebyrd/indri/internal/clients/sqlite"
	"github.com/robbiebyrd/indri/internal/models"
	repoErrors "github.com/robbiebyrd/indri/internal/repo"
	"github.com/robbiebyrd/indri/internal/services/events"
	"github.com/robbiebyrd/indri/internal/services/lock"
)

func newSQLiteFixture(t *testing.T) *SQLiteStore {
	t.Helper()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	store, err := NewSQLiteStore(context.Background(), db, lock.NewInProcess(), events.NewInProcess())
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return store
}

func TestNewSQLiteStore_ReturnsUsableStore(t *testing.T) {
	store := newSQLiteFixture(t)
	if store == nil {
		t.Fatal("NewSQLiteStore returned nil without error")
	}
}

func TestSQLiteStore_NewGame_AssignsIDAndCode(t *testing.T) {
	store := newSQLiteFixture(t)
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

func TestSQLiteStore_NewGame_DuplicateCode_ReturnsErrDuplicate(t *testing.T) {
	store := newSQLiteFixture(t)
	if _, err := store.New("ABCD", makeScript(), false); err != nil {
		t.Fatalf("first New: %v", err)
	}
	_, err := store.New("ABCD", makeScript(), false)
	if !errors.Is(err, repoErrors.ErrDuplicate) {
		t.Fatalf("expected ErrDuplicate, got %v", err)
	}
}

func TestSQLiteStore_Get_UnknownID_ReturnsErrNotFound(t *testing.T) {
	store := newSQLiteFixture(t)
	_, err := store.Get("no-such-id")
	if !errors.Is(err, repoErrors.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestSQLiteStore_FindByCode(t *testing.T) {
	store := newSQLiteFixture(t)
	created, _ := store.New("WXYZ", makeScript(), false)

	got, err := store.FindByCode("WXYZ")
	if err != nil {
		t.Fatalf("FindByCode: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("ID: want %q, got %q", created.ID, got.ID)
	}
}

func TestSQLiteStore_Exists(t *testing.T) {
	store := newSQLiteFixture(t)
	created, _ := store.New("ABCD", makeScript(), false)

	exists, _ := store.Exists(created.Code)
	if !exists {
		t.Errorf("expected exists=true for created code")
	}
	exists, _ = store.Exists("nope")
	if exists {
		t.Errorf("expected exists=false for unknown code")
	}
}

func TestSQLiteStore_FindOpen_ExcludesPrivate(t *testing.T) {
	store := newSQLiteFixture(t)
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

func TestSQLiteStore_Update_BumpsVersion(t *testing.T) {
	store := newSQLiteFixture(t)
	g, _ := store.New("ABCD", makeScript(), false)

	upd := &models.UpdateGame{Private: true}
	if err := store.Update(g.ID, upd); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ := store.Get(g.ID)
	if !got.Private || got.Version != 2 {
		t.Errorf("Version=%d Private=%v; want 2, true", got.Version, got.Private)
	}
}

func TestSQLiteStore_UpdateField(t *testing.T) {
	store := newSQLiteFixture(t)
	g, _ := store.New("ABCD", makeScript(), false)

	if err := store.UpdateField(g.ID, "data.foo", "bar"); err != nil {
		t.Fatalf("UpdateField: %v", err)
	}
	got, _ := store.Get(g.ID)
	if v, _ := got.PublicData["foo"]; v != "bar" {
		t.Errorf("data.foo = %v; want bar", v)
	}
}

func TestSQLiteStore_DeleteField(t *testing.T) {
	store := newSQLiteFixture(t)
	g, _ := store.New("ABCD", makeScript(), false)
	_ = store.UpdateField(g.ID, "data.foo", "bar")

	if err := store.DeleteField(g.ID, "data.foo"); err != nil {
		t.Fatalf("DeleteField: %v", err)
	}
	got, _ := store.Get(g.ID)
	if _, still := got.PublicData["foo"]; still {
		t.Errorf("data.foo not deleted")
	}
}

func TestSQLiteStore_Mutate_SuccessfulApply(t *testing.T) {
	store := newSQLiteFixture(t)
	g, _ := store.New("ABCD", makeScript(), false)

	err := store.Mutate(g.ID, func(game *models.Game) error {
		game.Private = true
		return nil
	})
	if err != nil {
		t.Fatalf("Mutate: %v", err)
	}
	got, _ := store.Get(g.ID)
	if !got.Private || got.Version != 2 {
		t.Errorf("Mutate did not apply or bump version: %+v", got)
	}
}

func TestSQLiteStore_Mutate_UnknownID(t *testing.T) {
	store := newSQLiteFixture(t)
	err := store.Mutate("no-such-id", func(g *models.Game) error { return nil })
	if !errors.Is(err, repoErrors.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
