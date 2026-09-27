package game

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

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

	exists, _ := store.Exists(created.Code)
	if !exists {
		t.Errorf("expected exists=true for created code")
	}
	exists, _ = store.Exists("nope")
	if exists {
		t.Errorf("expected exists=false for unknown code")
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

// TestMemoryStore_ConcurrentSceneWritesAndReadsDoNotRace runs Mutates that
// write a scene's data alongside readers that walk the whole game. Run under
// -race: a copy that shares nested maps with stored state races here.
func TestMemoryStore_ConcurrentSceneWritesAndReadsDoNotRace(t *testing.T) {
	store := newMemoryFixture(t)
	g, err := store.New("RACE", sceneScript(), false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	const n = 50
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if err := store.Mutate(g.ID, func(g *models.Game) error {
				(*g.Stage.Scenes["s1"].PublicData)["n"] = i
				return nil
			}); err != nil {
				t.Errorf("Mutate: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			got, err := store.Get(g.ID)
			if err != nil {
				t.Errorf("Get: %v", err)
				return
			}
			if _, err := events.ToMap(got); err != nil {
				t.Errorf("ToMap: %v", err)
			}
		}()
	}
	wg.Wait()
}

// readingPublisher reads the game back while publishing, as the broadcaster
// does when a change asks for a keyframe.
type readingPublisher struct {
	store *MemoryStore
}

func (p *readingPublisher) Publish(_ context.Context, e events.ChangeEvent) error {
	_, err := p.store.Get(e.ID)
	return err
}

func (p *readingPublisher) Subscribe(context.Context) (<-chan events.ChangeEvent, error) {
	return make(chan events.ChangeEvent), nil
}

// TestMemoryStore_PublishesOutsideTheStoreLock proves a subscriber may read
// the store while a change is being published: publishing under the store's
// lock would deadlock that read.
func TestMemoryStore_PublishesOutsideTheStoreLock(t *testing.T) {
	pub := &readingPublisher{}
	store, err := NewMemoryStore(context.Background(), lock.NewInProcess(), pub)
	if err != nil {
		t.Fatalf("NewMemoryStore: %v", err)
	}
	pub.store = store

	g, err := store.New("LOCK", makeScript(), false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- store.UpdateField(g.ID, "data.foo", "bar") }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("UpdateField: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("UpdateField deadlocked: the change was published while holding the store lock")
	}
}
