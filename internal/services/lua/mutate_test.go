package lua

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	lua "github.com/yuin/gopher-lua"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/repo/game"
	"github.com/robbiebyrd/indri/internal/services/events"
)

// testTimeout bounds anything that could hang rather than fail. The nested
// indri.mutate test is the reason it exists: the bug it guards against is a
// deadlock on the game's keyed mutex, and a deadlocked test that simply never
// returns tells a reader nothing.
const testTimeout = 5 * time.Second

// capturingPublisher records every delta the store fans out.
//
// The deltas are the observable half of a mutation — a write that publishes
// nothing is invisible to players — so "performed no write" and "published
// exactly these paths" are both assertions about what arrives here.
type capturingPublisher struct {
	mu     sync.Mutex
	events []events.ChangeEvent
}

func (p *capturingPublisher) Publish(_ context.Context, event events.ChangeEvent) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.events = append(p.events, event)

	return nil
}

func (p *capturingPublisher) Subscribe(context.Context) (<-chan events.ChangeEvent, error) {
	return nil, nil
}

// updatedPaths returns every dotted path published so far, sorted.
//
// Sorted because UpdatedFields is a map and its iteration order is random: an
// unsorted comparison would pass or fail depending on the run.
func (p *capturingPublisher) updatedPaths() []string {
	p.mu.Lock()
	defer p.mu.Unlock()

	var paths []string

	for _, event := range p.events {
		for path := range event.UpdatedFields {
			paths = append(paths, path)
		}
	}

	slices.Sort(paths)

	return paths
}

func (p *capturingPublisher) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	return len(p.events)
}

// newTestGame builds a real in-memory store holding one game, and returns both.
//
// A MemoryStore rather than a stub: it runs the same core as the MongoDB store,
// so the lock, the version fence, the retry loop and the delta computation under
// test here are the production ones. A stub would have proved only that the test
// and the test agree.
func newTestGame(t *testing.T) (*game.MemoryStore, *capturingPublisher, string) {
	t.Helper()

	publisher := &capturingPublisher{}
	store := game.NewMemoryStore(context.Background(), nil, publisher)

	// The game starts with a data map rather than none. A game stamped from a
	// script that declares no data has PublicData nil, and a nil map is not a
	// document the store's dotted-path writes can descend into — so a fixture
	// without it would exercise "create the map" rather than the ordinary edit
	// these tests are about.
	g, err := store.New("ABCD", &models.Script{
		PublicData: map[string]interface{}{"round": 0},
	}, false)
	if err != nil {
		t.Fatalf("creating a game: %v", err)
	}

	return store, publisher, g.ID.Hex()
}

// invokeMove loads src, dispatches "move" against gameID and returns the error
// the invocation produced, if any.
func invokeMove(t *testing.T, games GameMutator, gameID, src string) error {
	t.Helper()

	e := newTestEngineWithGames(t, games, writeScript(t, "game.lua", src))

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	_, err := splitScriptError(e.Invoke(ctx, "move", actions.Request{
		Session: &models.Session{UserID: stringPtr("player-1"), GameID: &gameID},
	}))

	return err
}

// mustInvokeMove is invokeMove, failing the test on any error.
func mustInvokeMove(t *testing.T, games GameMutator, gameID, src string) {
	t.Helper()

	if err := invokeMove(t, games, gameID, src); err != nil {
		t.Fatalf("the script failed: %v", err)
	}
}

// --- the write itself --------------------------------------------------------

// A script's edit must land in the store, and must land as a delta naming only
// what it changed. Publishing more than changed would make every script edit
// look like a full refresh to every client in the game.
func TestMutate_CommitsAChangeAndPublishesOnlyIt(t *testing.T) {
	store, publisher, id := newTestGame(t)

	mustInvokeMove(t, store, id, `
indri.on("move", function(req)
  return indri.mutate(function(state)
    state.data = state.data or {}
    state.data.round = 7
    return state
  end)
end)
`)

	stored, err := store.Get(id)
	if err != nil {
		t.Fatalf("reloading the game: %v", err)
	}

	if got := stored.PublicData["round"]; got != float64(7) {
		t.Fatalf("data.round = %v (%T), want 7 — the script's edit did not commit", got, got)
	}

	// data.round is the script's edit, as a leaf path rather than a replacement
	// of the whole data map. updatedAt rides along because the store stamps it
	// on every committed save; it is part of any write, script or not.
	want := []string{"data.round", "updatedAt"}
	if got := publisher.updatedPaths(); !equalStrings(got, want) {
		t.Fatalf("published %v, want exactly %v", got, want)
	}
}

// indri.mutate answers whether anything was written. That is the signal the
// effect ledger is built on, so it has to be true for a commit and false for
// both flavours of no-op.
func TestMutate_ReportsWhetherItCommitted(t *testing.T) {
	tests := map[string]struct {
		body string
		want string
	}{
		"a change commits": {
			body: `state.data = state.data or {}; state.data.round = 1; return state`,
			want: "true",
		},
		"returning nil does not": {
			body: `return nil`,
			want: "false",
		},
		"returning the state unchanged does not": {
			body: `return state`,
			want: "false",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			store, _, id := newTestGame(t)

			// Reported by raising, which is all a script can do to speak today.
			err := invokeMove(t, store, id, `
indri.on("move", function(req)
  local committed = indri.mutate(function(state)
    `+test.body+`
  end)
  error("committed:" .. tostring(committed), 0)
end)
`)

			if err == nil {
				t.Fatal("the script returned without reporting")
			}

			if want := "committed:" + test.want; !strings.Contains(err.Error(), want) {
				t.Fatalf("the script reported %q, want it to contain %q", err.Error(), want)
			}
		})
	}
}

// --- the two no-ops ----------------------------------------------------------

// Returning nil is how a script says "I looked, and there is nothing to do" —
// the common case for a move that changes no state. It must cost neither a
// write nor a delta.
func TestMutate_ReturningNilWritesNothing(t *testing.T) {
	store, publisher, id := newTestGame(t)

	before := versionOf(t, store, id)

	mustInvokeMove(t, store, id, `
indri.on("move", function(req)
  return indri.mutate(function(state) return nil end)
end)
`)

	if got := versionOf(t, store, id); got != before {
		t.Fatalf("version moved from %d to %d, but the script returned nil", before, got)
	}

	if got := publisher.count(); got != 0 {
		t.Fatalf("published %d events for a script that returned nil, want 0", got)
	}
}

// A script that reads the state, decides nothing needs changing and hands the
// same state back is indistinguishable in intent from one returning nil. It is
// much easier to write by accident, so it must be just as free.
func TestMutate_ReturningAnUnchangedStateWritesNothing(t *testing.T) {
	store, publisher, id := newTestGame(t)

	before := versionOf(t, store, id)

	mustInvokeMove(t, store, id, `
indri.on("move", function(req)
  return indri.mutate(function(state) return state end)
end)
`)

	if got := versionOf(t, store, id); got != before {
		t.Fatalf("version moved from %d to %d, but the state was returned unchanged", before, got)
	}

	if got := publisher.count(); got != 0 {
		t.Fatalf("published %d events for an unchanged state, want 0", got)
	}
}

// --- failure leaves nothing behind -------------------------------------------

// A script that errors part-way through has already edited the table it was
// given. None of that may reach the store: the mutation must unwind with the
// document untouched and nothing published, or a buggy script would corrupt a
// game halfway through a move.
func TestMutate_AScriptErrorWritesNothingAndPublishesNothing(t *testing.T) {
	store, publisher, id := newTestGame(t)

	before := versionOf(t, store, id)

	err := invokeMove(t, store, id, `
indri.on("move", function(req)
  return indri.mutate(function(state)
    state.data = state.data or {}
    state.data.round = 99
    error("the script gave up", 0)
  end)
end)
`)

	if err == nil {
		t.Fatal("the failing script reported success")
	}

	if !strings.Contains(err.Error(), "the script gave up") {
		t.Fatalf("the error %q does not carry the script's own message", err.Error())
	}

	if got := versionOf(t, store, id); got != before {
		t.Fatalf("version moved from %d to %d after a failed script", before, got)
	}

	stored, getErr := store.Get(id)
	if getErr != nil {
		t.Fatalf("reloading the game: %v", getErr)
	}

	// Compared as text: the seeded value arrives as a BSON integer while a Lua
	// write would arrive as a float, and the point is that it is still the
	// former.
	if got := fmt.Sprint(stored.PublicData["round"]); got != "0" {
		t.Fatalf("data.round = %v, want the seeded 0 — the failed script's edit reached the document", got)
	}

	if got := publisher.count(); got != 0 {
		t.Fatalf("published %d events for a failed script, want 0", got)
	}
}

// --- the nested call ---------------------------------------------------------

// mutation.Run holds a lock keyed on the game, and lock.InProcess is a plain
// keyed mutex with no notion of which goroutine holds it. A nested
// indri.mutate would therefore wait on a lock its own caller holds and never
// wake up. Raising is the only outcome that is not a hung request, and the
// deadline on this test is what tells a failure from a hang.
func TestMutate_NestedCallRaisesRatherThanDeadlocking(t *testing.T) {
	store, _, id := newTestGame(t)

	done := make(chan error, 1)

	go func() {
		done <- invokeMove(t, store, id, `
indri.on("move", function(req)
  return indri.mutate(function(outer)
    return indri.mutate(function(inner) return inner end)
  end)
end)
`)
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a nested indri.mutate was allowed")
		}

		if !strings.Contains(err.Error(), "inside another indri.mutate") {
			t.Fatalf("the error %q does not explain the nesting rule", err.Error())
		}
	case <-time.After(testTimeout):
		t.Fatal("a nested indri.mutate deadlocked instead of raising")
	}
}

// A second, sequential indri.mutate in the same handler is legal: the guard is
// against nesting, not against mutating twice. Without this the guard could be
// implemented as a once-per-invocation latch and still pass the test above.
func TestMutate_TwoSequentialCallsAreAllowed(t *testing.T) {
	store, _, id := newTestGame(t)

	mustInvokeMove(t, store, id, `
indri.on("move", function(req)
  indri.mutate(function(state)
    state.data = state.data or {}
    state.data.first = 1
    return state
  end)
  indri.mutate(function(state)
    state.data = state.data or {}
    state.data.second = 2
    return state
  end)
end)
`)

	stored, err := store.Get(id)
	if err != nil {
		t.Fatalf("reloading the game: %v", err)
	}

	if stored.PublicData["first"] != float64(1) || stored.PublicData["second"] != float64(2) {
		t.Fatalf("data = %v, want both writes present", stored.PublicData)
	}
}

// --- the retry loop ----------------------------------------------------------

// conflictOnce loses the version fence exactly once, by writing to the game
// behind the first attempt's back.
//
// It wraps the real store rather than replacing it, so the conflict is resolved
// by the real retry loop and the real fence. UpdateField is safe to call from
// inside apply: it is a single-field write that takes no lock, so it cannot
// deadlock against the lock this mutation already holds.
type conflictOnce struct {
	inner *game.MemoryStore
	fired bool
}

func (c *conflictOnce) MutateResult(
	ctx context.Context,
	id string,
	apply func(g *models.Game) error,
) (bool, error) {
	return c.inner.MutateResult(ctx, id, func(g *models.Game) error {
		if !c.fired {
			c.fired = true

			if err := c.inner.UpdateField(id, "data.interloper", true); err != nil {
				return err
			}
		}

		return apply(g)
	})
}

// A version-fence miss reloads the game and runs the callback again. The
// callback must see the *reloaded* state, not the one the losing attempt
// edited — otherwise a retried mutation silently doubles its own effect.
//
// The script increments a counter that starts from whatever is stored. Run
// twice against reloaded state it commits 1; run twice against the same carried
// -over table it would commit 2. That difference is the whole assertion.
func TestMutate_AConflictRerunsTheCallbackAgainstReloadedState(t *testing.T) {
	store, _, id := newTestGame(t)
	games := &conflictOnce{inner: store}

	mustInvokeMove(t, games, id, `
indri.on("move", function(req)
  return indri.mutate(function(state)
    state.data = state.data or {}
    state.data.attempts = (state.data.attempts or 0) + 1
    return state
  end)
end)
`)

	if !games.fired {
		t.Fatal("the test never forced a conflict, so it proved nothing")
	}

	stored, err := store.Get(id)
	if err != nil {
		t.Fatalf("reloading the game: %v", err)
	}

	if got := stored.PublicData["attempts"]; got != float64(1) {
		t.Fatalf("data.attempts = %v, want 1 — the retry did not reload the state", got)
	}

	// Proof the retry happened at all rather than the first attempt simply
	// winning: the interloper's write is only visible to a reloaded attempt.
	if got := stored.PublicData["interloper"]; got != true {
		t.Fatalf("data.interloper = %v, want true — the committing attempt did not see the reload", got)
	}
}

// --- refusals ----------------------------------------------------------------

// The game a script edits comes from the authenticated session and never from
// the message, which is the rule kick is this repo's reference implementation
// of. A payload naming someone else's game must be ignored outright — not
// merely ranked below the session, because a caller who is in no game would
// then have nothing outranking it.
//
// This is the test that distinguishes "reads the session" from "reads the
// session first": without it, a payload fallback added underneath the session
// check passes every other test in this file.
func TestMutate_IgnoresAGameIdInThePayload(t *testing.T) {
	store, publisher, id := newTestGame(t)

	e := newTestEngineWithGames(t, store, writeScript(t, "game.lua", `
indri.on("move", function(req)
  return indri.mutate(function(state)
    state.data = state.data or {}
    state.data.round = 99
    return state
  end)
end)
`))

	// A caller in no game, naming a real one they have no claim to.
	_, err := splitScriptError(e.Invoke(context.Background(), "move", actions.Request{
		Session: &models.Session{UserID: stringPtr("outsider")},
		Payload: map[string]interface{}{"gameId": id, "code": "ABCD"},
	}))

	if err == nil {
		t.Fatal("a caller with no game edited a game named in their own payload")
	}

	if !strings.Contains(err.Error(), "not in a game") {
		t.Fatalf("the error %q does not say the caller has no game", err.Error())
	}

	if got := publisher.count(); got != 0 {
		t.Fatalf("published %d events for a refused edit, want 0", got)
	}

	stored, getErr := store.Get(id)
	if getErr != nil {
		t.Fatalf("reloading the game: %v", getErr)
	}

	if got := fmt.Sprint(stored.PublicData["round"]); got != "0" {
		t.Fatalf("data.round = %v, want the seeded 0 — the outsider's edit landed", got)
	}
}

// A script must not be able to edit a game its caller is not in. The game is
// taken from the authenticated session and never from the payload, so an
// unauthenticated or unjoined caller has no game at all and the call is refused
// rather than defaulted.
func TestMutate_RefusesACallerWithNoGame(t *testing.T) {
	store, _, _ := newTestGame(t)

	e := newTestEngineWithGames(t, store, writeScript(t, "game.lua", `
indri.on("move", function(req)
  return indri.mutate(function(state) return state end)
end)
`))

	tests := map[string]*models.Session{
		"no session at all":         nil,
		"a session not in any game": {UserID: stringPtr("player-1")},
	}

	for name, session := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := splitScriptError(e.Invoke(context.Background(), "move", actions.Request{Session: session}))
			if err == nil {
				t.Fatal("indri.mutate ran for a caller with no game")
			}

			if !strings.Contains(err.Error(), "not in a game") {
				t.Fatalf("the error %q does not say the caller has no game", err.Error())
			}
		})
	}
}

// An engine built without a store must say so plainly rather than dereferencing
// nil. Tests build one routinely, and a production misconfiguration should read
// as a sentence and not a stack trace.
func TestMutate_RefusesWhenTheEngineHasNoStore(t *testing.T) {
	err := invokeMove(t, nil, "some-game", `
indri.on("move", function(req)
  return indri.mutate(function(state) return state end)
end)
`)

	if err == nil {
		t.Fatal("indri.mutate ran with no game store")
	}

	if !strings.Contains(err.Error(), "without a game store") {
		t.Fatalf("the error %q does not name the missing store", err.Error())
	}
}

// --- isolation between invocations -------------------------------------------

// clearInvocation is what makes the call context not outlive its call.
//
// Tested directly rather than through two Invokes, because Invoke installs a
// fresh context before every handler runs: a second invocation overwrites the
// first's rather than reading it, so an end-to-end test passes whether or not
// the clearing happens at all. The guarantee is only observable here — and it
// is the one that matters, since a state goes back to the pool between calls
// and must carry no caller's game or deadline with it.
func TestClearInvocation_LeavesNoContextBehind(t *testing.T) {
	store, _, id := newTestGame(t)

	L := lua.NewState()
	t.Cleanup(L.Close)

	setInvocation(L, &invocation{ctx: context.Background(), gameID: id, games: store})

	if _, err := currentInvocation(L); err != nil {
		t.Fatalf("the context was not installed: %v", err)
	}

	clearInvocation(L)

	inv, err := currentInvocation(L)
	if err == nil {
		t.Fatalf("the state still holds a context for game %q after the call ended", inv.gameID)
	}

	if !strings.Contains(err.Error(), "no request in progress") {
		t.Fatalf("the error %q does not say there is no request in progress", err.Error())
	}
}

// A pooled state serves many requests, and each must be given its own call
// context rather than inheriting the last one. This is the end-to-end half;
// TestClearInvocation_LeavesNoContextBehind covers the clearing itself.
func TestMutate_EachInvocationGetsItsOwnCallContext(t *testing.T) {
	store, _, id := newTestGame(t)

	e := newTestEngineWithGames(t, store, writeScript(t, "game.lua", `
indri.on("move", function(req)
  return indri.mutate(function(state) return state end)
end)
`))

	if _, err := splitScriptError(e.Invoke(context.Background(), "move", actions.Request{
		Session: &models.Session{UserID: stringPtr("player-1"), GameID: &id},
	})); err != nil {
		t.Fatalf("the first invocation failed: %v", err)
	}

	// The same pooled state, now serving a caller who is in no game. If the
	// context leaked, this would quietly edit the previous caller's game.
	_, err := splitScriptError(e.Invoke(context.Background(), "move", actions.Request{
		Session: &models.Session{UserID: stringPtr("player-2")},
	}))
	if err == nil {
		t.Fatal("the second invocation reused the first one's game")
	}

	if !strings.Contains(err.Error(), "not in a game") {
		t.Fatalf("the error %q does not say the caller has no game", err.Error())
	}
}

// --- helpers -----------------------------------------------------------------

func versionOf(t *testing.T, store *game.MemoryStore, id string) int64 {
	t.Helper()

	g, err := store.Get(id)
	if err != nil {
		t.Fatalf("reloading the game: %v", err)
	}

	return g.Version
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}

	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}

	return true
}
