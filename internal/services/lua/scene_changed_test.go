package lua

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	lua "github.com/yuin/gopher-lua"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/repo/game"
)

// The scenes a test game is stamped with. A script moving between them is the
// live way a game's current scene changes — the Go call site the plan named,
// stage.Service.SetCurrentScene, is constructed nowhere — so this is where
// scene:changed has to come from.
const (
	lobbyScene = "lobby"
	boardScene = "board"
	finalScene = "final"
)

// newSceneGame builds a real in-memory store holding one game that is on a
// scene.
//
// A MemoryStore rather than a stub, for the reason newTestGame uses one: the
// lock, the version fence, the retry loop and the published delta under test
// here are the production ones.
func newSceneGame(t *testing.T) (*game.MemoryStore, *capturingPublisher, string) {
	t.Helper()

	publisher := &capturingPublisher{}
	store := game.NewMemoryStore(context.Background(), nil, publisher)

	g, err := store.New("ABCD", &models.Script{
		PublicData: map[string]interface{}{"round": 0},
		Stage: models.Stage{
			CurrentScene: lobbyScene,
			Scenes: map[string]models.Scene{
				lobbyScene: {},
				boardScene: {},
				finalScene: {},
			},
		},
	}, false)
	if err != nil {
		t.Fatalf("creating a game: %v", err)
	}

	return store, publisher, g.ID.Hex()
}

// sceneScript is a "move" action whose mutate callback is body, plus a
// scene:changed subscriber that appends what it was told to the game's own
// public data.
//
// The subscriber writes through indri.mutate rather than reporting some other
// way, for the reason recordScript does: it proves in one act that the handler
// ran, that it could reach the store, and that what it saw is what the mutation
// did.
func sceneScript(body string) string {
	return fmt.Sprintf(`
indri.on("move", function(req)
  indri.mutate(function(state)
%s
    return state
  end)
end)

indri.on("scene:changed", function(ev)
  indri.mutate(function(state)
    state.data.log = (state.data.log or "") .. ev.sceneId .. "<-" .. tostring(ev.previous) .. ";"
    return state
  end)
end)
`, body)
}

// moveTo dispatches the "move" action carrying a scene, and returns whatever the
// invocation failed with.
func moveTo(t *testing.T, e *Engine, gameID, scene string) error {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	_, err := splitScriptError(e.Invoke(ctx, "move", actions.Request{
		Session: &models.Session{UserID: stringPtr("player-1"), GameID: &gameID},
		Payload: map[string]interface{}{"scene": scene},
	}))

	return err
}

// readGame reads the stored game back, which is the only honest way to tell a
// handler that never ran from one that ran and wrote nothing.
func readGame(t *testing.T, store *game.MemoryStore, id string) *models.Game {
	t.Helper()

	g, err := store.Get(id)
	if err != nil {
		t.Fatalf("reading the game back: %v", err)
	}

	return g
}

// --- what raises the event ----------------------------------------------------

// A script that moves the game to another scene raises scene:changed, and one
// that writes stage.currentScene without moving it raises nothing.
//
// The event is read off the two document views the delta is diffed on, so what a
// subscriber is told and what the broadcast says are the same move by
// construction: one mutate callback, however many times it assigned, is one
// change and at most one event.
func TestMutate_RaisesSceneChangedWhenAScriptMovesTheScene(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		body  string
		want  string
		scene string
	}{
		{
			name:  "moves to another scene",
			body:  fmt.Sprintf("    state.stage.currentScene = %q", boardScene),
			want:  boardScene + "<-" + lobbyScene + ";",
			scene: boardScene,
		},
		{
			name:  "sets the scene it is already on",
			body:  fmt.Sprintf("    state.stage.currentScene = %q", lobbyScene),
			want:  "",
			scene: lobbyScene,
		},
		{
			name:  "changes something that is not the scene",
			body:  "    state.data.round = state.data.round + 1",
			want:  "",
			scene: lobbyScene,
		},
		{
			name: "moves twice before returning",
			body: fmt.Sprintf("    state.stage.currentScene = %q\n    state.stage.currentScene = %q",
				boardScene, finalScene),
			want:  finalScene + "<-" + lobbyScene + ";",
			scene: finalScene,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			store, _, id := newSceneGame(t)
			e := newTestEngineWithGames(t, store, writeScript(t, "game.lua", sceneScript(test.body)))

			if err := moveTo(t, e, id, test.scene); err != nil {
				t.Fatalf("dispatching move: %v", err)
			}

			g := readGame(t, store, id)

			log, _ := g.PublicData["log"].(string)
			if log != test.want {
				t.Fatalf("the scene:changed subscriber logged %q, want %q", log, test.want)
			}

			if got := g.Stage.CurrentScene; got != test.scene {
				t.Fatalf("the game is on scene %q, want %q", got, test.scene)
			}
		})
	}
}

// A script that never subscribed hears nothing, and — because it is checked
// before the event is described — costs nothing either.
//
// This is the common case: a server whose scripts care about one event out of
// four. It is also what keeps the chain cap below unreachable on such a server,
// since the depth only ever grows through a subscriber.
func TestMutate_RaisesNothingWhenNoScriptSubscribed(t *testing.T) {
	t.Parallel()

	store, publisher, id := newSceneGame(t)
	e := newTestEngineWithGames(t, store, writeScript(t, "game.lua", fmt.Sprintf(`
indri.on("move", function(req)
  indri.mutate(function(state)
    state.stage.currentScene = %q
    return state
  end)
end)
`, boardScene)))

	if err := moveTo(t, e, id, boardScene); err != nil {
		t.Fatalf("dispatching move: %v", err)
	}

	if got := readGame(t, store, id).Stage.CurrentScene; got != boardScene {
		t.Fatalf("the game is on scene %q, want %q: the move itself did not land", got, boardScene)
	}

	// One delta: the move. A subscriber would have written a second.
	if got := publisher.count(); got != 1 {
		t.Fatalf("published %d deltas, want 1: something ran for an event nobody subscribed to", got)
	}
}

// --- the ledger ---------------------------------------------------------------

// fenceLoser is a real store whose *first* mutation loses its first attempt to
// the version fence.
//
// It interferes rather than pretends: the write it performs between the script's
// callback and the store's save is an ordinary UpdateField, which bumps the
// version exactly as another player's move would, so the retry that follows is
// the store's own. Everything below it — the lock, the fence, the retry budget,
// the published delta — is the production path.
//
// Only the first mutation, because the subscriber the scene change wakes runs a
// mutation of its own through the same store, and making that one lose a fence
// too would be testing the retry loop twice rather than the ledger once.
type fenceLoser struct {
	*game.MemoryStore

	// mutations counts the mutations run through this store, and attempts counts
	// the attempts the first of them took.
	mutations int
	attempts  int
}

func (f *fenceLoser) MutateResult(
	ctx context.Context,
	id string,
	apply func(g *models.Game) error,
) (bool, error) {
	f.mutations++
	first := f.mutations == 1

	return f.MemoryStore.MutateResult(ctx, id, func(g *models.Game) error {
		if err := apply(g); err != nil {
			return err
		}

		if !first {
			return nil
		}

		f.attempts++

		if f.attempts > 1 {
			return nil
		}

		// A writer that got in between this attempt's load and its save. It does
		// not take the game's lock — UpdateField is a single conditional write —
		// so it can commit while this mutation holds it, which is the whole point.
		return f.MemoryStore.UpdateField(id, "data.interference", true)
	})
}

// A scene change made by an attempt that lost the version fence is discarded
// with that attempt, and only the scene the store actually holds is announced.
//
// The script picks a different scene on each attempt — it can tell them apart by
// the interfering write — so the losing attempt describes a move to "final" that
// never happened. A subscriber told about it would be told about a state nobody
// stored, and the delta broadcast beside it would say something else.
func TestSceneChanged_IsDiscardedWithTheAttemptThatLostTheFence(t *testing.T) {
	t.Parallel()

	store, _, id := newSceneGame(t)
	games := &fenceLoser{MemoryStore: store}

	e := newTestEngineWithGames(t, games, writeScript(t, "game.lua", sceneScript(fmt.Sprintf(`
    if state.data.interference == nil then
      state.stage.currentScene = %q
    else
      state.stage.currentScene = %q
    end`, finalScene, boardScene))))

	if err := moveTo(t, e, id, boardScene); err != nil {
		t.Fatalf("dispatching move: %v", err)
	}

	if games.attempts != 2 {
		t.Fatalf("the mutation ran %d attempt(s), want 2: the fence was never lost", games.attempts)
	}

	g := readGame(t, store, id)

	if got := g.Stage.CurrentScene; got != boardScene {
		t.Fatalf("the game is on scene %q, want %q", got, boardScene)
	}

	want := boardScene + "<-" + lobbyScene + ";"
	if log, _ := g.PublicData["log"].(string); log != want {
		t.Fatalf("the subscriber was told %q, want %q: a losing attempt's scene change was raised", log, want)
	}
}

// --- what bounds a chain of scene changes -------------------------------------

// sceneDepthMark is one observation a running handler made of the chain it is
// part of.
type sceneDepthMark struct {
	// running is the scene depth the invocation making the observation is at.
	running int

	// queued is the depth of the scene change that invocation has queued on its
	// ledger, or -1 when it queued none.
	queued int
}

type sceneDepthLog struct {
	mu    sync.Mutex
	marks []sceneDepthMark
}

func (s *sceneDepthLog) add(mark sceneDepthMark) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.marks = append(s.marks, mark)
}

func (s *sceneDepthLog) all() []sceneDepthMark {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]sceneDepthMark{}, s.marks...)
}

// sceneDepthCapability is indri.probe.mark(), which reports the chain depth of
// the invocation that called it and of the scene change that invocation queued.
//
// White-box, deliberately. The bound on a chain of scene changes is a number
// that rides on a context and on a ledger, and neither is anything a script can
// see; observing it from inside a real invocation is what makes "the handler for
// an event at depth d runs at depth d, and queues the next at d+1" an assertion
// rather than an inference from a chain having stopped.
func sceneDepthCapability(marks *sceneDepthLog) capabilityInstaller {
	return func(L *lua.LState) (lua.LValue, error) {
		tbl := L.NewTable()

		tbl.RawSetString("mark", L.NewFunction(func(L *lua.LState) int {
			inv, err := currentInvocation(L)
			if err != nil {
				L.RaiseError("probe.mark: %s", err.Error())
			}

			marks.add(sceneDepthMark{
				running: sceneDepthFrom(inv.ctx),
				queued:  queuedSceneDepth(&inv.effects),
			})

			return 0
		}))

		return tbl, nil
	}
}

// queuedSceneDepth reports the depth of the scene change on a ledger, or -1 when
// there is none. It walks both levels through count, so an effect queued inside
// a mutate that has already committed is found where it now is.
func queuedSceneDepth(l *ledger) int {
	depth := -1

	l.count(func(e effect) bool {
		if scene, ok := e.(sceneChangedEffect); ok {
			depth = scene.depth
		}

		return false
	})

	return depth
}

// newSceneDepthEngine builds an engine whose only capability is the depth probe.
func newSceneDepthEngine(t *testing.T, marks *sceneDepthLog, games GameMutator, path string) *Engine {
	t.Helper()

	e, err := newEngine(
		[]models.ScriptFile{{Path: path, Grants: []string{probeName}}},
		games,
		capabilitySet{probeName: {install: sceneDepthCapability(marks), inAction: true}},
	)
	if err != nil {
		t.Fatalf("building an engine over %v: %v", path, err)
	}

	t.Cleanup(e.Close)

	return e
}

// chainScript is a scene:changed subscriber that moves the scene again, which is
// the recursion this bound exists for, and reports the depth it saw.
//
// The valve is not the subject and must not be mistaken for it. Without it, a
// build whose bound had been removed would loop forever and this test would hang
// rather than fail, which is the one outcome that teaches a reader nothing. Its
// limit is four times the cap, so it can only ever be reached by a build that is
// already broken.
const chainScript = `
indri.on("scene:changed", function(ev)
  indri.mutate(function(state)
    state.data.round = (state.data.round or 0) + 1
    if state.data.round > 40 then return nil end
    state.stage.currentScene = "scene-" .. tostring(state.data.round)
    return state
  end)
  indri.probe.mark()
end)
`

// A scene:changed handler that moves the scene again is bounded, and this is the
// derivation the bound rests on rather than the fact that a chain stopped.
//
// Each link is its own invocation, on its own pooled state, reached through the
// queue: the two share no object at all. What ties them together is one line in
// emit, which installs the event's depth into the invocation that handles it, and
// one in queueSceneChange, which reads it back and queues the next link at one
// more. Removing either — emit builds a fresh context on purpose, so dropping the
// install reads like tidying up — leaves every link looking like the first, and
// an A→B→A ping-pong draining forever with nothing to stop it: not the deadline,
// which is fresh per event, and not maxPendingLifecycle, which a chain one link
// wide never reaches.
//
// Each case therefore asserts the depth of every link, starting from an event
// already partway down a chain. Under the substitution every mark reads zero and
// the second one fails, in milliseconds.
//
// Not parallel: the chain ends in a refusal, which is logged.
func TestSceneChanged_CarriesTheChainDepthIntoTheHandler(t *testing.T) {
	for _, start := range []int{0, 1, 5, maxSceneChangeDepth - 1} {
		t.Run(fmt.Sprintf("from depth %d", start), func(t *testing.T) {
			logged := captureLog(t)

			marks := &sceneDepthLog{}
			store, _, id := newSceneGame(t)
			e := newSceneDepthEngine(t, marks, store, writeScript(t, "game.lua", chainScript))

			e.emitLifecycleAt(LifecycleSceneChanged, id, LifecycleSubject{
				"sceneId":  boardScene,
				"previous": lobbyScene,
			}, start)

			// One link per remaining depth. The link at the cap itself leaves no
			// mark: its indri.mutate raises, so the handler unwinds before it can
			// report.
			want := make([]sceneDepthMark, 0, maxSceneChangeDepth-start)
			for depth := start; depth < maxSceneChangeDepth; depth++ {
				want = append(want, sceneDepthMark{running: depth, queued: depth + 1})
			}

			got := marks.all()

			if len(got) != len(want) {
				t.Fatalf("the chain ran %d handler(s), want %d: %v", len(got), len(want), got)
			}

			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("link %d ran at %+v, want %+v: the chain depth is not carried between links",
						i, got[i], want[i])
				}
			}

			// The chain ended because the cap refused the last move, not because
			// the script's own valve tripped.
			if want := fmt.Sprintf("%s depth %d exceeded", LifecycleSceneChanged, maxSceneChangeDepth); !strings.Contains(logged.String(), want) {
				t.Fatalf("the log does not mention %q; it says %q", want, logged.String())
			}
		})
	}
}

// moverScript is a scene:changed subscriber that moves the game on one more
// scene — the recursion the cap exists for — and stops once it is already there,
// so a chain of it ends on its own if it is allowed to.
const moverScript = `
indri.on("scene:changed", function(ev)
  indri.mutate(function(state)
    if state.stage.currentScene == "final" then return nil end
    state.stage.currentScene = "final"
    return state
  end)
end)
`

// At the cap the move itself is refused, not merely its announcement.
//
// Letting the last write through while quietly telling nobody would leave the
// game on a scene no subscriber ever heard about — a state the players' rebuilt
// view and the server's disagree on, which is worse than a move that failed.
// The script hears about it at its own line, and the operator in the log.
//
// Not parallel: it reads the standard logger.
func TestSceneChanged_RefusesTheMoveAtTheChainCap(t *testing.T) {
	logged := captureLog(t)

	store, publisher, id := newSceneGame(t)
	e := newTestEngineWithGames(t, store, writeScript(t, "game.lua", moverScript))

	e.emitLifecycleAt(LifecycleSceneChanged, id, LifecycleSubject{
		"sceneId":  boardScene,
		"previous": lobbyScene,
	}, maxSceneChangeDepth)

	if got := readGame(t, store, id).Stage.CurrentScene; got != lobbyScene {
		t.Fatalf("the game moved to %q at the chain cap, want it left on %q", got, lobbyScene)
	}

	if got := publisher.count(); got != 0 {
		t.Fatalf("a refused move published %d deltas, want 0", got)
	}

	for _, want := range []string{
		fmt.Sprintf("%s depth %d exceeded", LifecycleSceneChanged, maxSceneChangeDepth),
		"has to stop itself",
	} {
		if !strings.Contains(logged.String(), want) {
			t.Fatalf("the log does not mention %q; it says %q", want, logged.String())
		}
	}
}

// A move one link below the cap is allowed, and the link it queues is handled.
// The chain is the runaway, not the depth: a game that legitimately advances ten
// scenes in one act must still work, and a bound that refused the tenth would
// bound games rather than loops.
func TestSceneChanged_AllowsTheMoveBelowTheChainCap(t *testing.T) {
	t.Parallel()

	store, _, id := newSceneGame(t)
	e := newTestEngineWithGames(t, store, writeScript(t, "game.lua", moverScript))

	e.emitLifecycleAt(LifecycleSceneChanged, id, LifecycleSubject{
		"sceneId":  boardScene,
		"previous": lobbyScene,
	}, maxSceneChangeDepth-1)

	if got := readGame(t, store, id).Stage.CurrentScene; got != finalScene {
		t.Fatalf("the game is on scene %q, want %q: the last permitted move was refused", got, finalScene)
	}
}
