package lua

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
)

// recordScript subscribes to one event and writes everything it was handed into
// the game's own public data.
//
// Writing through indri.mutate rather than reporting some other way is
// deliberate: it proves the two things a lifecycle handler is for in one act —
// that it can reach the game store at all, and that what it saw is what the
// emitter sent. The value is read back out of the store afterwards, so a
// handler that never ran and a handler that ran and wrote nothing are not the
// same result.
func recordScript(event string, fields ...string) string {
	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		parts = append(parts, fmt.Sprintf("tostring(ev.%s)", field))
	}

	return fmt.Sprintf(`
indri.on(%q, function(ev)
  indri.mutate(function(state)
    state.data.seen = table.concat({%s}, "|")
    return state
  end)
end)
`, event, strings.Join(parts, ", "))
}

// emitAndRead raises one event against a real in-memory store and returns
// whatever the handler recorded.
func emitAndRead(t *testing.T, src string, ev lifecycleEvent) string {
	t.Helper()

	store, _, id := newTestGame(t)
	e := newTestEngineWithGames(t, store, writeScript(t, "game.lua", src))

	ev.gameID = id
	e.EmitLifecycle(ev.event, ev.gameID, ev.subject)

	g, err := store.Get(id)
	if err != nil {
		t.Fatalf("reading the game back: %v", err)
	}

	seen, _ := g.PublicData["seen"].(string)

	return seen
}

// --- what a handler may subscribe to ----------------------------------------

// A script subscribes to a lifecycle event with the same indri.on it registers
// an action with, and the four events are the ones the server raises.
func TestOn_AcceptsEveryLifecycleEvent(t *testing.T) {
	t.Parallel()

	for _, event := range lifecycleEvents {
		t.Run(event, func(t *testing.T) {
			t.Parallel()

			e := newTestEngine(t, writeScript(t, "game.lua",
				fmt.Sprintf("indri.on(%q, function() end)", event)))

			if got := e.Lifecycle(); !slices.Equal(got, []string{event}) {
				t.Fatalf("Lifecycle() = %v, want [%s]", got, event)
			}
		})
	}
}

// An unknown lifecycle name is refused at load, not at emit time.
//
// This is the whole reason the colon marks a namespace. A misspelled event that
// registered successfully would be a handler that compiles, boots, and silently
// never runs — the failure mode a script author has no way to see. Refusing it
// while the chunk is loading turns it into a boot failure naming the file, the
// line and the events that do exist.
func TestOn_RefusesAnUnknownLifecycleName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		event string
	}{
		{name: "plausible but not raised", event: "player:quit"},
		{name: "defined by a later story", event: "game:ended"},
		{name: "right subject wrong verb", event: "scene:advanced"},
		{name: "nothing but the mark", event: ":"},
		{name: "arbitrary namespace", event: "myGame:somethingHappened"},
		{name: "correct name, wrong case", event: "Player:Joined"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			path := writeScript(t, "game.lua", fmt.Sprintf("indri.on(%q, function() end)", test.event))

			_, err := NewEngine([]string{path}, nil)
			requireErrorMentions(t, err, "game.lua", test.event, "not a lifecycle event", LifecyclePlayerJoined)
		})
	}
}

// A lifecycle event is not an action, and the separation is structural rather
// than a check somebody has to remember.
//
// Everything dispatchable is built from Actions(): the router registrations, the
// REST route table, the GraphQL mutations and the set of names a timer may
// fire. An event that appeared there would be reachable by any client that
// could spell it — "player:left" aimed at a player who is winning.
func TestLifecycle_IsNotADispatchableAction(t *testing.T) {
	t.Parallel()

	e := newTestEngine(t, writeScript(t, "game.lua", `
indri.on("move", function() end)
indri.on("player:joined", function() end)
`))

	if got := e.Actions(); !slices.Equal(got, []string{"move"}) {
		t.Fatalf("Actions() = %v, want [move]: a lifecycle event must not be dispatchable", got)
	}

	_, err := e.Invoke(context.Background(), LifecyclePlayerJoined, actions.Request{})
	requireErrorMentions(t, err, "no lua handler is registered", LifecyclePlayerJoined)
}

// The reverse of the separation: an action name may not carry the lifecycle
// mark, so a script cannot register an action that an emission would find.
func TestOn_RefusesAnActionCarryingTheLifecycleMark(t *testing.T) {
	t.Parallel()

	path := writeScript(t, "game.lua", `indri.on("my:action", function() end)`)

	_, err := NewEngine([]string{path}, nil)
	requireErrorMentions(t, err, "not a lifecycle event")
}

// --- what a handler is handed ------------------------------------------------

// Each event carries the game id and its own subject.
func TestEmitLifecycle_CarriesTheGameAndTheSubject(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		event   string
		subject LifecycleSubject
		fields  []string
		want    string
	}{
		{
			name:    "game created",
			event:   LifecycleGameCreated,
			subject: LifecycleSubject{"code": "ABCD"},
			fields:  []string{"event", "code"},
			want:    "game:created|ABCD",
		},
		{
			name:    "player joined",
			event:   LifecyclePlayerJoined,
			subject: LifecycleSubject{"userId": "player-1", "teamId": "red"},
			fields:  []string{"event", "userId", "teamId"},
			want:    "player:joined|player-1|red",
		},
		{
			name:    "player left",
			event:   LifecyclePlayerLeft,
			subject: LifecycleSubject{"userId": "player-2"},
			fields:  []string{"event", "userId"},
			want:    "player:left|player-2",
		},
		{
			name:    "scene changed",
			event:   LifecycleSceneChanged,
			subject: LifecycleSubject{"sceneId": "board", "previous": "lobby"},
			fields:  []string{"event", "sceneId", "previous"},
			want:    "scene:changed|board|lobby",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := emitAndRead(t,
				recordScript(test.event, test.fields...),
				lifecycleEvent{event: test.event, subject: test.subject},
			)

			if got != test.want {
				t.Fatalf("the handler saw %q, want %q", got, test.want)
			}
		})
	}
}

// The game id is always there, whatever the subject says, and it is the field a
// handler reaches its game by — the same field a timer-fired action reads,
// because neither has a session to read it from.
func TestEmitLifecycle_AlwaysCarriesTheGameID(t *testing.T) {
	t.Parallel()

	store, _, id := newTestGame(t)
	e := newTestEngineWithGames(t, store, writeScript(t, "game.lua",
		recordScript(LifecyclePlayerLeft, "gameId")))

	e.EmitLifecycle(LifecyclePlayerLeft, id, nil)

	g, err := store.Get(id)
	if err != nil {
		t.Fatalf("reading the game back: %v", err)
	}

	if got := g.PublicData["seen"]; got != id {
		t.Fatalf("ev.gameId = %v, want the emitted game %v", got, id)
	}
}

// A subject may not redefine which event this is or which game it belongs to.
// The two fields the host owns are written last precisely so a careless — or
// hostile — subject key cannot forge them.
func TestEmitLifecycle_SubjectCannotForgeTheHostFields(t *testing.T) {
	t.Parallel()

	store, _, id := newTestGame(t)
	e := newTestEngineWithGames(t, store, writeScript(t, "game.lua",
		recordScript(LifecyclePlayerJoined, "event", "gameId")))

	e.EmitLifecycle(LifecyclePlayerJoined, id, LifecycleSubject{
		"event":  "game:created",
		"gameId": "some-other-game",
	})

	g, err := store.Get(id)
	if err != nil {
		t.Fatalf("reading the game back: %v", err)
	}

	want := LifecyclePlayerJoined + "|" + id
	if got := g.PublicData["seen"]; got != want {
		t.Fatalf("the handler saw %v, want %q", got, want)
	}
}

// A lifecycle event carries no session, the same contract a timer fires under.
//
// Nobody is connected when a game is created or a player drops, and inventing a
// session would hand a subscriber authority no player granted it. Absent rather
// than empty, so a script asking who caused this has to say what it means for
// the answer to be nobody.
func TestEmitLifecycle_CarriesNoSession(t *testing.T) {
	t.Parallel()

	got := emitAndRead(t,
		recordScript(LifecyclePlayerJoined, "session"),
		lifecycleEvent{event: LifecyclePlayerJoined, subject: LifecycleSubject{"userId": "player-1"}},
	)

	if got != "nil" {
		t.Fatalf("ev.session = %q, want nil", got)
	}
}

// --- what a handler may do ---------------------------------------------------

// A lifecycle handler can call indri.mutate, and the write lands in the store
// like any other. The store here is a real MemoryStore, so the lock, the
// version fence, the retry budget and the published delta are the production
// ones.
func TestEmitLifecycle_HandlerCanMutate(t *testing.T) {
	t.Parallel()

	store, publisher, id := newTestGame(t)
	e := newTestEngineWithGames(t, store, writeScript(t, "game.lua", `
indri.on("player:joined", function(ev)
  indri.mutate(function(state)
    state.data.round = state.data.round + 1
    state.data.welcomed = ev.userId
    return state
  end)
end)
`))

	e.EmitLifecycle(LifecyclePlayerJoined, id, LifecycleSubject{"userId": "player-1"})

	g, err := store.Get(id)
	if err != nil {
		t.Fatalf("reading the game back: %v", err)
	}

	if got := asNumber(t, g.PublicData["round"]); got != 1 {
		t.Fatalf("data.round = %v, want 1: the handler's mutate did not commit", got)
	}

	// The subject reaches inside the callback, which is the whole shape a
	// subscriber is written in: read the event, edit the game it happened to.
	if got := g.PublicData["welcomed"]; got != "player-1" {
		t.Fatalf("data.welcomed = %v, want player-1", got)
	}

	// A write that never publishes is invisible to players, and a lifecycle
	// handler's write is no exception.
	if got := publisher.updatedPaths(); !slices.Contains(got, "data.round") {
		t.Fatalf("published %v, want a delta naming data.round", got)
	}
}

// --- failures -----------------------------------------------------------------

// A lifecycle handler that raises must not take anything else down with it.
//
// EmitLifecycle returns nothing at all, so a subscriber cannot fail the
// built-in action that raised the event — that is the contract, enforced by the
// signature rather than by a caller remembering to ignore an error. What this
// asserts is the two halves a signature cannot: the failure is reported to the
// operator rather than swallowed, and the drain survives, so the event behind a
// failing one is still told.
//
// Not parallel: it reads the standard logger.
func TestEmitLifecycle_AFailingHandlerIsLoggedAndDoesNotStopTheNextEvent(t *testing.T) {
	logged := captureLog(t)

	store, _, id := newTestGame(t)
	e := newTestEngineWithGames(t, store, writeScript(t, "game.lua", `
indri.on("player:joined", function(ev)
  error("this subscriber is broken", 0)
end)

indri.on("player:left", function(ev)
  indri.mutate(function(state)
    state.data.seen = "left ran anyway"
    return state
  end)
end)
`))

	e.EmitLifecycle(LifecyclePlayerJoined, id, LifecycleSubject{"userId": "player-1"})
	e.EmitLifecycle(LifecyclePlayerLeft, id, LifecycleSubject{"userId": "player-1"})

	g, err := store.Get(id)
	if err != nil {
		t.Fatalf("reading the game back: %v", err)
	}

	if got := g.PublicData["seen"]; got != "left ran anyway" {
		t.Fatalf("data.seen = %v: a raising handler stopped the events behind it", got)
	}

	if want := "this subscriber is broken"; !strings.Contains(logged.String(), want) {
		t.Fatalf("the log does not mention %q; it says %q", want, logged.String())
	}
}

// A handler that raises inside indri.mutate leaves the game as it found it: the
// mutation unwinds without writing, exactly as it does for an action.
func TestEmitLifecycle_AFailedMutateWritesNothing(t *testing.T) {
	t.Parallel()

	store, publisher, id := newTestGame(t)
	e := newTestEngineWithGames(t, store, writeScript(t, "game.lua", `
indri.on("player:joined", function(ev)
  indri.mutate(function(state)
    state.data.round = 99
    error("changed my mind", 0)
  end)
end)
`))

	e.EmitLifecycle(LifecyclePlayerJoined, id, LifecycleSubject{"userId": "player-1"})

	g, err := store.Get(id)
	if err != nil {
		t.Fatalf("reading the game back: %v", err)
	}

	if got := asNumber(t, g.PublicData["round"]); got != 0 {
		t.Fatalf("data.round = %v, want 0: a handler that unwound wrote anyway", got)
	}

	if got := publisher.count(); got != 0 {
		t.Fatalf("published %d deltas for a mutation that never committed, want 0", got)
	}
}

// An event nobody subscribed to costs nothing and does nothing. This is the
// common case on a server whose scripts care about one event out of four, and
// it must not reach a Lua state at all.
func TestEmitLifecycle_IgnoresAnEventNobodySubscribedTo(t *testing.T) {
	t.Parallel()

	store, publisher, id := newTestGame(t)
	e := newTestEngineWithGames(t, store, writeScript(t, "game.lua",
		recordScript(LifecyclePlayerJoined, "event")))

	e.EmitLifecycle(LifecycleSceneChanged, id, LifecycleSubject{"sceneId": "board"})
	e.EmitLifecycle(LifecycleGameCreated, id, LifecycleSubject{"code": "ABCD"})

	if got := publisher.count(); got != 0 {
		t.Fatalf("an unsubscribed event published %d deltas, want 0", got)
	}
}

// An emission this server cannot make is refused, and says so.
//
// Both cases can only come from inside this repo — the names are Go constants
// and the game id comes from the store — so both are wiring mistakes rather
// than a script's, and a silent refusal would be an event that simply never
// arrives with nothing anywhere to say why. Not parallel: it reads the standard
// logger, which is process-wide.
func TestEmitLifecycle_RefusesWhatItCannotRaise(t *testing.T) {
	logged := captureLog(t)

	store, publisher, id := newTestGame(t)
	e := newTestEngineWithGames(t, store, writeScript(t, "game.lua",
		recordScript(LifecyclePlayerJoined, "event")))

	// A name no version of this server raises.
	e.EmitLifecycle("player:quit", id, nil)

	// The right name with no game: a handler reaches its game through
	// indri.mutate, which has nothing to fall back on, so this could only raise.
	e.EmitLifecycle(LifecyclePlayerJoined, "", LifecycleSubject{"userId": "player-1"})

	if got := publisher.count(); got != 0 {
		t.Fatalf("an unknown or gameless event published %d deltas, want 0", got)
	}

	for _, want := range []string{`unknown lifecycle event "player:quit"`, `"player:joined" with no game`} {
		if !strings.Contains(logged.String(), want) {
			t.Fatalf("the log does not mention %q; it says %q", want, logged.String())
		}
	}
}

// captureLog redirects the standard logger into a buffer for the test's
// duration. The caller must not be parallel: the logger is process-wide.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer

	flags := log.Flags()
	out := log.Writer()

	log.SetOutput(&buf)
	log.SetFlags(0)

	t.Cleanup(func() {
		log.SetOutput(out)
		log.SetFlags(flags)
	})

	return &buf
}

// --- the deferred queue ------------------------------------------------------

// Nothing is dispatched inline by a goroutine that is already inside a drain.
//
// This is the property the whole feature rests on, and it is asserted on the
// queue itself because the hand-off is what a test of the engine would only see
// indirectly: the second caller must be told it does not own the drain, so its
// event lands on the drain already running rather than starting a nested one.
func TestLifecycleQueue_DefersWhileADrainIsRunning(t *testing.T) {
	t.Parallel()

	var q lifecycleQueue

	own, dropped := q.enqueue(lifecycleEvent{event: LifecycleGameCreated})
	if !own || dropped {
		t.Fatalf("the first enqueue returned own=%v dropped=%v, want true/false", own, dropped)
	}

	// Raised from inside the drain the first caller owns.
	if own, _ := q.enqueue(lifecycleEvent{event: LifecyclePlayerJoined}); own {
		t.Fatal("a second enqueue claimed the drain while one was already running")
	}

	var drained []string

	for {
		ev, ok := q.next()
		if !ok {
			break
		}

		drained = append(drained, ev.event)
	}

	want := []string{LifecycleGameCreated, LifecyclePlayerJoined}
	if !slices.Equal(drained, want) {
		t.Fatalf("the drain handled %v, want %v in that order", drained, want)
	}

	// Closed again once it ran dry, so the next emission opens a fresh drain
	// rather than queueing behind one nobody is running.
	if own, _ := q.enqueue(lifecycleEvent{event: LifecyclePlayerLeft}); !own {
		t.Fatal("an enqueue after the drain ran dry did not claim a new drain")
	}
}

// The queue drops rather than blocks once it is full. The caller is a built-in
// action that has already committed: making it wait on a script is the one
// outcome worse than losing an event and saying so.
func TestLifecycleQueue_DropsPastTheCap(t *testing.T) {
	t.Parallel()

	var q lifecycleQueue

	for i := range maxPendingLifecycle {
		if _, dropped := q.enqueue(lifecycleEvent{event: LifecycleGameCreated}); dropped {
			t.Fatalf("event %d of %d was dropped before the cap", i, maxPendingLifecycle)
		}
	}

	if _, dropped := q.enqueue(lifecycleEvent{event: LifecycleGameCreated}); !dropped {
		t.Fatalf("the %dth event was accepted, want it dropped", maxPendingLifecycle+1)
	}
}

// Events raised from several goroutines at once all arrive exactly once.
//
// Whichever goroutine finds the queue idle owns the drain and may end up
// running another's event; what must never happen is an event enqueued in the
// window where a drain is closing being left behind with nobody to run it.
func TestEmitLifecycle_LosesNothingUnderConcurrentEmitters(t *testing.T) {
	t.Parallel()

	const emitters = 16

	store, _, id := newTestGame(t)
	e := newTestEngineWithGames(t, store, writeScript(t, "game.lua", `
indri.on("player:joined", function(ev)
  indri.mutate(function(state)
    state.data.round = state.data.round + 1
    return state
  end)
end)
`))

	var wg sync.WaitGroup

	for i := range emitters {
		wg.Add(1)

		go func() {
			defer wg.Done()

			e.EmitLifecycle(LifecyclePlayerJoined, id, LifecycleSubject{
				"userId": fmt.Sprintf("player-%d", i),
			})
		}()
	}

	wg.Wait()

	// Every emitter has returned, so every drain it may have owned has run dry;
	// nothing is still in flight.
	g, err := store.Get(id)
	if err != nil {
		t.Fatalf("reading the game back: %v", err)
	}

	if got := asNumber(t, g.PublicData["round"]); got != emitters {
		t.Fatalf("data.round = %v after %d concurrent emissions, want %d", got, emitters, emitters)
	}
}

// asNumber reads a number out of stored game data, whatever numeric type the
// round trip through BSON and Lua left it as. A stored zero arrives as an int32
// and a value Lua wrote arrives as a float64, and a test comparing against one
// of those would pass or fail for a reason that is not its subject.
func asNumber(t *testing.T, value interface{}) float64 {
	t.Helper()

	switch n := value.(type) {
	case int:
		return float64(n)
	case int32:
		return float64(n)
	case int64:
		return float64(n)
	case float64:
		return n
	default:
		t.Fatalf("%#v is not a number", value)

		return 0
	}
}

// --- wiring -------------------------------------------------------------------

// The engine is what the game service raises events through, and this is the
// assignment that says so. A signature that drifted would otherwise be found at
// boot rather than at compile time.
var _ interface {
	EmitLifecycle(event, gameID string, subject map[string]interface{})
} = (*Engine)(nil)

// A lifecycle handler gets its script's own capabilities, exactly as an action
// handler does.
//
// A handler's environment is built fresh on every invocation and falls back to
// the *shared* host table, so a granted capability only survives into a call
// because bindScope wrapped the handler in its script's own table. Binding the
// actions alone would leave a subscriber finding nil where its grant should be
// — at the moment a game was created, which is the worst place to discover it.
func TestEmitLifecycle_HandlerKeepsItsScriptsCapabilities(t *testing.T) {
	t.Parallel()

	calls := &probeLog{}

	path := writeScript(t, "game.lua", `
indri.on("player:joined", function(ev)
  indri.probe.mark("joined:" .. ev.userId)
end)
`)

	e := newProbeEngine(t, calls, granted(path, probeName))

	e.EmitLifecycle(LifecyclePlayerJoined, "game-1", LifecycleSubject{"userId": "player-1"})

	if got := calls.all(); !slices.Equal(got, []string{"joined:player-1"}) {
		t.Fatalf("the granted capability recorded %v, want [joined:player-1]", got)
	}
}
