package game

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/services/events"
	"github.com/robbiebyrd/indri/internal/services/lock"
)

func newMemoryStoreWith(t *testing.T, publisher events.Publisher) *MemoryStore {
	t.Helper()

	store, err := NewMemoryStore(context.Background(), lock.NewInProcess(), publisher)
	if err != nil {
		t.Fatal(err)
	}

	return store
}

// editPath round-trips the game through JSON; fields hidden from JSON must
// survive, or a field write would un-delete a game or reset its version.
func TestEditPath_PreservesFieldsHiddenFromJSON(t *testing.T) {
	deleted := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	g := &models.Game{ID: "g1", Version: 7, DeletedAt: deleted, PublicData: map[string]interface{}{}}

	if err := editPath(g, "data.score", 3.0, false); err != nil {
		t.Fatal(err)
	}

	if g.Version != 7 || !g.DeletedAt.Equal(deleted) {
		t.Fatalf("hidden fields lost: version=%d deletedAt=%v", g.Version, g.DeletedAt)
	}
	if g.PublicData["score"] != 3.0 {
		t.Fatalf("data.score = %v, want 3", g.PublicData["score"])
	}

	if err := editPath(g, "data.score", nil, true); err != nil {
		t.Fatal(err)
	}
	if _, ok := g.PublicData["score"]; ok {
		t.Fatal("data.score still present after delete")
	}
}

func TestNewGame_PreDeclaresSlotsFromScript(t *testing.T) {
	g, _ := newGame("C1", &models.Script{
		Config: models.Config{MaxPlayersPerTeam: 2},
		Teams:  map[string]models.Team{"Red": {Name: "Red"}, "Blue": {Name: "Blue"}},
	}, true)

	if g.ID == "" || g.Version != 1 || g.Code != "C1" || !g.Private {
		t.Fatalf("header = %+v", g)
	}
	if len(g.Players) != 4 || len(g.Teams["Blue"].PlayerIDs) != 2 || len(g.Teams["Red"].PlayerIDs) != 2 {
		t.Fatalf("slots not pre-declared: players=%v teams=%v", g.Players, g.Teams)
	}
}

func TestSlotOperations(t *testing.T) {
	store := newMemoryStoreWith(t, events.NewInProcess())

	g, err := store.New("SLOTS", &models.Script{
		Config: models.Config{MaxPlayersPerTeam: 1},
		Teams:  map[string]models.Team{"A": {Name: "A"}},
	}, false)
	if err != nil {
		t.Fatal(err)
	}

	slot, err := store.AssignSlot(g.ID, "A", "u1", "Alice")
	if err != nil {
		t.Fatal(err)
	}

	got, _ := store.Get(g.ID)
	p := got.Players[slot]
	if p.UserID != "u1" || !p.Connected || !p.Host {
		t.Fatalf("assigned slot = %+v, want u1, connected, host", p)
	}

	if _, err := store.AssignSlot(g.ID, "A", "u2", "Bob"); err == nil {
		t.Fatal("assigning into a full team succeeded")
	}
	if _, err := store.AssignSlot(g.ID, "nope", "u2", "Bob"); err == nil {
		t.Fatal("assigning into an unknown team succeeded")
	}

	if err := store.DisconnectPlayer(g.ID, slot, "u1"); err != nil {
		t.Fatal(err)
	}
	got, _ = store.Get(g.ID)
	if got.Players[slot].Connected {
		t.Fatal("still connected after DisconnectPlayer")
	}

	if err := store.RemovePlayer(g.ID, slot, "u1"); err != nil {
		t.Fatal(err)
	}
	got, _ = store.Get(g.ID)
	if pl, ok := got.Players[slot]; !ok || pl.UserID != "" {
		t.Fatalf("after RemovePlayer slot = %+v (present=%v), want an empty slot kept in place", pl, ok)
	}

	if err := store.ConnectPlayer(g.ID, "p99", "u1"); err == nil {
		t.Fatal("connecting a nonexistent slot succeeded")
	}
}

// A missing argument must be reported by its own name, so the error points at
// what the caller actually left out.
func TestSlotOperations_NameTheMissingArgument(t *testing.T) {
	store := newMemoryStoreWith(t, events.NewInProcess())

	operations := map[string]func(id, slotId string) error{
		"RemovePlayer":     func(id, slotId string) error { return store.RemovePlayer(id, slotId, "u1") },
		"ConnectPlayer":    func(id, slotId string) error { return store.ConnectPlayer(id, slotId, "u1") },
		"DisconnectPlayer": func(id, slotId string) error { return store.DisconnectPlayer(id, slotId, "u1") },
	}

	for name, operation := range operations {
		t.Run(name, func(t *testing.T) {
			if err := operation("", "p1"); err == nil || err.Error() != "game id is required" {
				t.Errorf("empty game id: err = %v, want %q", err, "game id is required")
			}
			if err := operation("g1", ""); err == nil || err.Error() != "slot id is required" {
				t.Errorf("empty slot id: err = %v, want %q", err, "slot id is required")
			}
		})
	}
}

// Every game gets its own copy of the script's data: games created from one
// script must never share state (the memory store keeps live references, so
// aliasing made later games start with earlier games' boards).
func TestNewGame_DoesNotShareStateWithTheScriptOrOtherGames(t *testing.T) {
	script := &models.Script{
		Config:     models.Config{MaxPlayersPerTeam: 1},
		Teams:      map[string]models.Team{"A": {Name: "A", PublicData: map[string]interface{}{"turn": true}}},
		PublicData: map[string]interface{}{"round": 1.0},
		Stage: models.Stage{CurrentScene: "board", Scenes: map[string]models.Scene{
			"board": {PublicData: &map[string]interface{}{"board": []interface{}{""}}},
		}},
	}

	store := newMemoryStoreWith(t, events.NewInProcess())
	first, _ := store.New("ONE", script, false)
	second, _ := store.New("TWO", script, false)

	if err := store.Mutate(first.ID, func(g *models.Game) error {
		(*g.Stage.Scenes["board"].PublicData)["board"] = []interface{}{"X"}
		g.PublicData["round"] = 2.0
		g.Teams["A"].PublicData["turn"] = false
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	got, _ := store.Get(second.ID)
	if b := (*got.Stage.Scenes["board"].PublicData)["board"]; !reflect.DeepEqual(b, []interface{}{""}) {
		t.Errorf("second game's board = %v; it shares state with the first", b)
	}
	if got.PublicData["round"] != 1.0 || got.Teams["A"].PublicData["turn"] != true {
		t.Errorf("second game's data changed with the first: round=%v turn=%v", got.PublicData["round"], got.Teams["A"].PublicData["turn"])
	}
	if (*script.Stage.Scenes["board"].PublicData)["board"].([]interface{})[0] != "" || script.PublicData["round"] != 1.0 {
		t.Error("playing a game modified the script")
	}
}
