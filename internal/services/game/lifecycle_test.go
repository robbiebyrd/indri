package game

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/robbiebyrd/indri/internal/models"
	gameRepo "github.com/robbiebyrd/indri/internal/repo/game"
	luaService "github.com/robbiebyrd/indri/internal/services/lua"
)

// raised is one lifecycle event the service raised, together with what the game
// looked like at the moment it was raised.
//
// The snapshot is the point. "The event carried the right fields" is the easy
// half; the half that matters is that a subscriber reading the store at that
// instant finds the write that caused the event already there. A recorder that
// only kept the arguments would pass just as happily if the event were raised
// before the commit.
type raised struct {
	event   string
	gameID  string
	subject map[string]interface{}
	game    *models.Game
	readErr error
}

// recorder is a LifecycleEmitter that reads the game back as it is called.
type recorder struct {
	store *gameRepo.MemoryStore

	mu     sync.Mutex
	events []raised
}

func (r *recorder) EmitLifecycle(event, gameID string, subject map[string]interface{}) {
	g, err := r.store.Get(gameID)

	r.mu.Lock()
	defer r.mu.Unlock()

	r.events = append(r.events, raised{
		event:   event,
		gameID:  gameID,
		subject: subject,
		game:    g,
		readErr: err,
	})
}

func (r *recorder) names() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	names := make([]string, 0, len(r.events))
	for _, e := range r.events {
		names = append(names, e.event)
	}

	return names
}

// only returns the single event of this kind, failing unless there is exactly
// one. "Raised exactly once" is an assertion in its own right: a join that
// raised player:joined twice would deal a second hand.
func (r *recorder) only(t *testing.T, event string) raised {
	t.Helper()

	r.mu.Lock()
	defer r.mu.Unlock()

	var found []raised

	for _, e := range r.events {
		if e.event == event {
			found = append(found, e)
		}
	}

	if len(found) != 1 {
		t.Fatalf("%s was raised %d times, want exactly once (all: %v)", event, len(found), r.events)
	}

	return found[0]
}

// newService builds the game service over a real in-memory store.
//
// A MemoryStore rather than a stub: it runs the same core as the MongoDB store,
// so the lock, the version fence, the retry loop and the change deltas these
// events are sequenced against are the production ones. Constructed as a struct
// literal rather than through NewService because the script store NewService
// reads is a MongoDB-backed repo and these tests need no script at all.
func newService(t *testing.T) (*Service, *recorder) {
	t.Helper()

	store := gameRepo.NewMemoryStore(context.Background(), nil, nil)
	events := &recorder{store: store}

	return &Service{gameRepo: store, Lifecycle: events}, events
}

// newGame creates a game and returns its id, discarding whatever the creation
// itself raised.
func newGame(t *testing.T, gs *Service, events *recorder) string {
	t.Helper()

	g, err := gs.New("ABCD", false)
	if err != nil {
		t.Fatalf("creating a game: %v", err)
	}

	events.mu.Lock()
	events.events = nil
	events.mu.Unlock()

	return g.ID.Hex()
}

// --- game:created -------------------------------------------------------------

// Creating a game raises game:created, carrying the game id and the code, and
// raises it only once the game is stored.
func TestNew_RaisesGameCreatedAfterTheInsert(t *testing.T) {
	t.Parallel()

	gs, events := newService(t)

	g, err := gs.New("ABCD", false)
	if err != nil {
		t.Fatalf("creating a game: %v", err)
	}

	got := events.only(t, luaService.LifecycleGameCreated)

	if got.gameID != g.ID.Hex() {
		t.Fatalf("game:created carried game %q, want %q", got.gameID, g.ID.Hex())
	}

	if got.subject["code"] != "ABCD" {
		t.Fatalf("game:created carried code %v, want ABCD", got.subject["code"])
	}

	// The after-commit assertion: the store already answers for this game at the
	// instant the event is raised.
	if got.readErr != nil || got.game == nil {
		t.Fatalf("a subscriber reading the game as it was told about it got %v/%v", got.game, got.readErr)
	}
}

// A creation that failed raises nothing. The code is already taken, so there is
// no new game for a subscriber to be told about.
func TestNew_RaisesNothingWhenTheCodeIsTaken(t *testing.T) {
	t.Parallel()

	gs, events := newService(t)

	newGame(t, gs, events)

	if _, err := gs.New("ABCD", false); err == nil {
		t.Fatal("creating a second game with the same code succeeded")
	}

	if got := events.names(); len(got) != 0 {
		t.Fatalf("a failed creation raised %v, want nothing", got)
	}
}

// --- player:joined -------------------------------------------------------------

// Joining raises player:joined once, with the player and the team, and only
// once the player is stored and on that team.
func TestConnectPlayer_RaisesPlayerJoinedAfterTheCommit(t *testing.T) {
	t.Parallel()

	gs, events := newService(t)
	id := newGame(t, gs, events)

	if err := gs.ConnectPlayer(id, "", "player-1", "Ada"); err != nil {
		t.Fatalf("connecting a player: %v", err)
	}

	got := events.only(t, luaService.LifecyclePlayerJoined)

	if got.gameID != id || got.subject["userId"] != "player-1" {
		t.Fatalf("player:joined carried %q/%v, want %q/player-1", got.gameID, got.subject["userId"], id)
	}

	if got.readErr != nil {
		t.Fatalf("a subscriber reading the game as it was told about it got %v", got.readErr)
	}

	// The write the event describes is already visible, which is the whole
	// contract: a subscriber must never see the game as it was before.
	if _, ok := got.game.Players["player-1"]; !ok {
		t.Fatalf("player:joined was raised before the player was stored; players = %v", got.game.Players)
	}
}

// A player who is already in the game joined once.
//
// A rejoin writes nothing — AddPlayer refuses a duplicate — so raising again
// would tell every subscriber that somebody joined on each reconnect. The store
// reports whether it committed; a nil error would not have distinguished these.
func TestConnectPlayer_RaisesNothingForAPlayerAlreadyInTheGame(t *testing.T) {
	t.Parallel()

	gs, events := newService(t)
	id := newGame(t, gs, events)

	for range 3 {
		if err := gs.ConnectPlayer(id, "", "player-1", "Ada"); err != nil {
			t.Fatalf("connecting a player: %v", err)
		}
	}

	// only() fails unless there is exactly one, which is the assertion.
	events.only(t, luaService.LifecyclePlayerJoined)
}

// --- player:left ---------------------------------------------------------------

// Leaving raises player:left once the player is gone from the stored game.
func TestRemovePlayer_RaisesPlayerLeftAfterTheCommit(t *testing.T) {
	t.Parallel()

	gs, events := newService(t)
	id := newGame(t, gs, events)

	if err := gs.ConnectPlayer(id, "", "player-1", "Ada"); err != nil {
		t.Fatalf("connecting a player: %v", err)
	}

	if err := gs.RemovePlayer(id, "player-1"); err != nil {
		t.Fatalf("removing a player: %v", err)
	}

	got := events.only(t, luaService.LifecyclePlayerLeft)

	if got.gameID != id || got.subject["userId"] != "player-1" {
		t.Fatalf("player:left carried %q/%v, want %q/player-1", got.gameID, got.subject["userId"], id)
	}

	if got.readErr != nil {
		t.Fatalf("a subscriber reading the game as it was told about it got %v", got.readErr)
	}

	if _, ok := got.game.Players["player-1"]; ok {
		t.Fatalf("player:left was raised before the player was removed; players = %v", got.game.Players)
	}
}

// Removing somebody who is not in the game is not a departure.
//
// The write still commits — the document simply does not change — so a commit
// flag alone would raise the event. A leave racing a kick for the same player
// must raise player:left once between them, not twice.
func TestRemovePlayer_RaisesNothingForSomebodyWhoWasNotThere(t *testing.T) {
	t.Parallel()

	gs, events := newService(t)
	id := newGame(t, gs, events)

	if err := gs.RemovePlayer(id, "never-joined"); err != nil {
		t.Fatalf("removing an absent player: %v", err)
	}

	if got := events.names(); len(got) != 0 {
		t.Fatalf("removing an absent player raised %v, want nothing", got)
	}
}

// A leave and a kick racing over the same player raise player:left exactly
// once. Only one of them can be why the player is gone, and the store's
// version fence is what decides which.
func TestRemovePlayer_RaisesOnceUnderARace(t *testing.T) {
	t.Parallel()

	gs, events := newService(t)
	id := newGame(t, gs, events)

	if err := gs.ConnectPlayer(id, "", "player-1", "Ada"); err != nil {
		t.Fatalf("connecting a player: %v", err)
	}

	var wg sync.WaitGroup

	for range 2 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			if err := gs.RemovePlayer(id, "player-1"); err != nil {
				t.Errorf("removing a player: %v", err)
			}
		}()
	}

	wg.Wait()

	events.only(t, luaService.LifecyclePlayerLeft)
}

// --- the service without an emitter ---------------------------------------------

// A server running no scripts has no emitter, and every call site must carry on
// exactly as it did before lifecycle events existed.
func TestService_WorksWithNoEmitter(t *testing.T) {
	t.Parallel()

	store := gameRepo.NewMemoryStore(context.Background(), nil, nil)
	gs := &Service{gameRepo: store}

	g, err := gs.New("ABCD", false)
	if err != nil {
		t.Fatalf("creating a game: %v", err)
	}

	if err := gs.ConnectPlayer(g.ID.Hex(), "", "player-1", "Ada"); err != nil {
		t.Fatalf("connecting a player: %v", err)
	}

	if err := gs.RemovePlayer(g.ID.Hex(), "player-1"); err != nil {
		t.Fatalf("removing a player: %v", err)
	}
}

// --- a real script on the other end ----------------------------------------------

// A script's bug cannot break join.
//
// The emitter here is the real engine running a real subscriber that raises on
// every event. ConnectPlayer must still report success and the player must
// still be in the game: the write committed before the event was raised, and a
// subscriber is not allowed to undo that by failing.
func TestLifecycleFailure_DoesNotFailTheBuiltInAction(t *testing.T) {
	t.Parallel()

	store := gameRepo.NewMemoryStore(context.Background(), nil, nil)

	path := filepath.Join(t.TempDir(), "broken.lua")
	if err := os.WriteFile(path, []byte(`
indri.on("game:created",  function(ev) error("broken subscriber", 0) end)
indri.on("player:joined", function(ev) error("broken subscriber", 0) end)
indri.on("player:left",   function(ev) error("broken subscriber", 0) end)
`), 0o600); err != nil {
		t.Fatalf("writing the script: %v", err)
	}

	engine, err := luaService.NewEngine([]string{path}, store)
	if err != nil {
		t.Fatalf("building the engine: %v", err)
	}

	t.Cleanup(engine.Close)

	gs := &Service{gameRepo: store, Lifecycle: engine}

	g, err := gs.New("ABCD", false)
	if err != nil {
		t.Fatalf("a raising game:created subscriber failed the create: %v", err)
	}

	if err := gs.ConnectPlayer(g.ID.Hex(), "", "player-1", "Ada"); err != nil {
		t.Fatalf("a raising player:joined subscriber failed the join: %v", err)
	}

	joined, err := store.Get(g.ID.Hex())
	if err != nil {
		t.Fatalf("reading the game back: %v", err)
	}

	if _, ok := joined.Players["player-1"]; !ok {
		t.Fatalf("the join was rolled back by a failing subscriber; players = %v", joined.Players)
	}

	if err := gs.RemovePlayer(g.ID.Hex(), "player-1"); err != nil {
		t.Fatalf("a raising player:left subscriber failed the leave: %v", err)
	}
}

// A subscriber that edits the game through indri.mutate is editing the state
// the built-in action just wrote, not the state before it — which is only true
// because the event is raised after the commit.
func TestLifecycleHandler_MutatesTheCommittedState(t *testing.T) {
	t.Parallel()

	store := gameRepo.NewMemoryStore(context.Background(), nil, nil)

	path := filepath.Join(t.TempDir(), "greeter.lua")
	if err := os.WriteFile(path, []byte(`
indri.on("player:joined", function(ev)
  indri.mutate(function(state)
    local roster = {}
    for id in pairs(state.players) do roster[#roster + 1] = id end
    table.sort(roster)

    state.data = state.data or {}
    state.data.roster = table.concat(roster, ",")
    return state
  end)
end)
`), 0o600); err != nil {
		t.Fatalf("writing the script: %v", err)
	}

	engine, err := luaService.NewEngine([]string{path}, store)
	if err != nil {
		t.Fatalf("building the engine: %v", err)
	}

	t.Cleanup(engine.Close)

	gs := &Service{gameRepo: store, Lifecycle: engine}

	g, err := gs.New("ABCD", false)
	if err != nil {
		t.Fatalf("creating a game: %v", err)
	}

	for _, player := range []string{"player-1", "player-2"} {
		if err := gs.ConnectPlayer(g.ID.Hex(), "", player, player); err != nil {
			t.Fatalf("connecting %s: %v", player, err)
		}
	}

	stored, err := store.Get(g.ID.Hex())
	if err != nil {
		t.Fatalf("reading the game back: %v", err)
	}

	// The second join's subscriber saw both players, which it could only do if
	// the second join had already committed when it ran.
	if got := stored.PublicData["roster"]; got != "player-1,player-2" {
		t.Fatalf("data.roster = %v, want player-1,player-2", got)
	}
}

// The event names this service raises are the ones the engine accepts. They are
// the engine's constants, so this is a statement about the vocabulary rather
// than a comparison of two spellings: a name the engine stopped raising would
// fail to compile here.
func TestLifecycleNames_AreOnesTheEngineRaises(t *testing.T) {
	t.Parallel()

	raisedHere := []string{
		luaService.LifecycleGameCreated,
		luaService.LifecyclePlayerJoined,
		luaService.LifecyclePlayerLeft,
	}

	for _, name := range raisedHere {
		path := filepath.Join(t.TempDir(), "sub.lua")
		if err := os.WriteFile(path, fmt.Appendf(nil, "indri.on(%q, function() end)", name), 0o600); err != nil {
			t.Fatalf("writing the script: %v", err)
		}

		engine, err := luaService.NewEngine([]string{path}, nil)
		if err != nil {
			t.Fatalf("a script subscribing to %q, which this service raises, failed to load: %v", name, err)
		}

		if got := engine.Lifecycle(); !slices.Equal(got, []string{name}) {
			t.Fatalf("subscribing to %q gave the manifest %v", name, got)
		}

		engine.Close()
	}
}
