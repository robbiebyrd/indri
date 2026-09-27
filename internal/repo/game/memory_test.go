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

func TestMemoryStore_Update_BumpsVersion(t *testing.T) {
	store := newMemoryFixture(t)
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

func TestMemoryStore_UpdateField(t *testing.T) {
	store := newMemoryFixture(t)
	g, _ := store.New("ABCD", makeScript(), false)

	if err := store.UpdateField(g.ID, "data.foo", "bar"); err != nil {
		t.Fatalf("UpdateField: %v", err)
	}
	got, _ := store.Get(g.ID)
	if v, _ := got.PublicData["foo"]; v != "bar" {
		t.Errorf("data.foo = %v; want bar", v)
	}
}

func TestMemoryStore_DeleteField(t *testing.T) {
	store := newMemoryFixture(t)
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

func TestMemoryStore_Mutate_SuccessfulApply(t *testing.T) {
	store := newMemoryFixture(t)
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

func TestMemoryStore_Mutate_UnknownID(t *testing.T) {
	store := newMemoryFixture(t)
	err := store.Mutate("no-such-id", func(g *models.Game) error { return nil })
	if !errors.Is(err, repoErrors.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestMemoryStore_AddPlayer_ThenHasPlayer(t *testing.T) {
	store := newMemoryFixture(t)
	g, _ := store.New("ABCD", makeScript(), false)

	if err := store.AddPlayer(g.ID, "user-1", "Alice"); err != nil {
		t.Fatalf("AddPlayer: %v", err)
	}
	if !store.HasPlayer(g.ID, "user-1") {
		t.Error("HasPlayer(user-1) = false")
	}
	if store.HasPlayer(g.ID, "user-x") {
		t.Error("HasPlayer(user-x) = true")
	}
	got, _ := store.Get(g.ID)
	if got.Players["user-1"].Name != "Alice" {
		t.Errorf("Name = %q; want Alice", got.Players["user-1"].Name)
	}
}

func TestMemoryStore_RemovePlayer(t *testing.T) {
	store := newMemoryFixture(t)
	g, _ := store.New("ABCD", makeScript(), false)
	_ = store.AddPlayer(g.ID, "user-1", "Alice")

	if err := store.RemovePlayer(g.ID, "user-1"); err != nil {
		t.Fatalf("RemovePlayer: %v", err)
	}
	if store.HasPlayer(g.ID, "user-1") {
		t.Error("HasPlayer(user-1) = true after remove")
	}
}

func TestMemoryStore_ConnectDisconnectPlayer(t *testing.T) {
	store := newMemoryFixture(t)
	g, _ := store.New("ABCD", makeScript(), false)
	_ = store.AddPlayer(g.ID, "user-1", "Alice")

	if err := store.ConnectPlayer(g.ID, "user-1"); err != nil {
		t.Fatalf("ConnectPlayer: %v", err)
	}
	got, _ := store.Get(g.ID)
	if !got.Players["user-1"].Connected {
		t.Errorf("Connected = false after ConnectPlayer")
	}

	if err := store.DisconnectPlayer(g.ID, "user-1"); err != nil {
		t.Fatalf("DisconnectPlayer: %v", err)
	}
	got, _ = store.Get(g.ID)
	if got.Players["user-1"].Connected {
		t.Errorf("Connected = true after DisconnectPlayer")
	}
}
