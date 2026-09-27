package game

import (
	"context"
	"errors"
	"testing"

	"github.com/robbiebyrd/indri/internal/models"
	repoErrors "github.com/robbiebyrd/indri/internal/repo"
	"github.com/robbiebyrd/indri/internal/services/events"
	"github.com/robbiebyrd/indri/internal/services/lock"
)

func newMemoryFixture(t *testing.T) *MemoryStore {
	t.Helper()
	store, err := NewMemoryStore(context.Background(), lock.NewInProcess(), events.NewInProcess())
	if err != nil {
		t.Fatalf("NewMemoryStore: %v", err)
	}
	return store
}

func makeScript() *models.Script {
	return &models.Script{
		Teams: map[string]models.Team{},
	}
}

func TestNewMemoryStore_ReturnsUsableStore(t *testing.T) {
	store := newMemoryFixture(t)
	if store == nil {
		t.Fatal("NewMemoryStore returned nil store without error")
	}
}

func TestMemoryStore_NewGame_AssignsIDAndCode(t *testing.T) {
	store := newMemoryFixture(t)
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

func TestMemoryStore_NewGame_DuplicateCode_ReturnsErrDuplicate(t *testing.T) {
	store := newMemoryFixture(t)
	if _, err := store.New("ABCD", makeScript(), false); err != nil {
		t.Fatalf("first New: %v", err)
	}
	_, err := store.New("ABCD", makeScript(), false)
	if !errors.Is(err, repoErrors.ErrDuplicate) {
		t.Fatalf("expected ErrDuplicate, got %v", err)
	}
}

func TestMemoryStore_Get_UnknownID_ReturnsErrNotFound(t *testing.T) {
	store := newMemoryFixture(t)
	_, err := store.Get("no-such-id")
	if !errors.Is(err, repoErrors.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestMemoryStore_Get_ReturnsDeepCopy(t *testing.T) {
	store := newMemoryFixture(t)
	created, _ := store.New("ABCD", makeScript(), false)

	got, err := store.Get(created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	got.Code = "MUTATED"

	fresh, _ := store.Get(created.ID)
	if fresh.Code != "ABCD" {
		t.Errorf("stored code mutated via returned pointer: got %q", fresh.Code)
	}
}

func TestMemoryStore_FindByCode(t *testing.T) {
	store := newMemoryFixture(t)
	created, _ := store.New("WXYZ", makeScript(), false)

	got, err := store.FindByCode("WXYZ")
	if err != nil {
		t.Fatalf("FindByCode: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("ID: want %q, got %q", created.ID, got.ID)
	}
}

func TestMemoryStore_Exists(t *testing.T) {
	store := newMemoryFixture(t)
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

func TestMemoryStore_FindOpen_ExcludesPrivate(t *testing.T) {
	store := newMemoryFixture(t)
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
