package lua

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
	"github.com/robbiebyrd/indri/internal/models"
)

// hookEngine builds an engine over one hook script and closes it with the test.
func hookEngine(t *testing.T, src string) *Engine {
	t.Helper()

	return newTestEngine(t, writeScript(t, "hooks.lua", src))
}

// invokeHookReply runs one hook and returns the single document it replied
// with, decoded.
//
// Replying rather than raising is how these tests read a hook's mind, because
// it is the one channel that proves the *success* path: a hook that raised
// would prove only that the closure was reached before it failed.
func invokeHookReply(
	t *testing.T,
	e *Engine,
	kind HookKind,
	action string,
	req actions.Request,
) map[string]interface{} {
	t.Helper()

	res, err := e.InvokeHook(context.Background(), kind, action, req)
	if err != nil {
		t.Fatalf("InvokeHook(%s, %q) = %v, want no error", kind, action, err)
	}

	if len(res.Responses) != 1 {
		t.Fatalf("the hook produced %d responses, want exactly the one it replied with", len(res.Responses))
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(res.Responses[0], &decoded); err != nil {
		t.Fatalf("the hook's reply %q is not a JSON object: %v", res.Responses[0], err)
	}

	return decoded
}

// --- what a script may hook -------------------------------------------------

// TestHookableActions_ExcludesEveryAuthAction is the security rule of this
// story stated as a test.
//
// Both directions are asserted. The exclusions matter because login's payload
// carries a plaintext password and reconnect's carries a bearer token, and a
// hook is the one mechanism that deliberately reaches a built-in. The
// inclusions matter because hookableActions is *derived* by subtraction: a
// typo in authActions that happened to match nothing would silently leave the
// list unchanged, and a check that only counted the exclusions would pass.
func TestHookableActions_ExcludesEveryAuthAction(t *testing.T) {
	t.Parallel()

	hookable := hookableActions()

	for _, action := range authActions {
		if slices.Contains(hookable, action) {
			t.Errorf("the credential action %q is hookable; a hook on it would be handed a password or a token", action)
		}

		if !slices.Contains(builtinActions, action) {
			t.Errorf("authActions names %q, which is not a built-in action at all; the exclusion is a no-op", action)
		}
	}

	for _, action := range builtinActions {
		if slices.Contains(authActions, action) {
			continue
		}

		if !slices.Contains(hookable, action) {
			t.Errorf("the built-in %q is neither hookable nor an auth action, so it is unreachable by either rule", action)
		}
	}

	if len(hookable) == 0 {
		t.Fatal("no action is hookable; the test is not checking anything")
	}
}

// TestHooks_RefusedAtLoad covers every name a script may not hook. Each is a
// boot failure rather than a hook that quietly never runs, because a game rule
// nobody enforces is the failure mode with no symptom.
func TestHooks_RefusedAtLoad(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		src   string
		wants []string
	}{
		"login carries a plaintext password": {
			src:   `indri.before("login", function(req) end)`,
			wants: []string{"login", "credentials"},
		},
		"register carries the password being set": {
			src:   `indri.after_action("register", function(req) end)`,
			wants: []string{"register", "credentials"},
		},
		"reconnect carries the bearer token": {
			src:   `indri.before("reconnect", function(req) end)`,
			wants: []string{"reconnect", "credentials"},
		},
		"logout invalidates the token": {
			src:   `indri.before("logout", function(req) end)`,
			wants: []string{"logout", "credentials"},
		},
		"a misspelled built-in": {
			src:   `indri.before("jion", function(req) end)`,
			wants: []string{"jion", "only a built-in action can be hooked"},
		},
		"a script's own action is not a built-in": {
			src: `
indri.on("move", function(req) end)
indri.before("move", function(req) end)
`,
			wants: []string{"move", "only a built-in action can be hooked"},
		},
		"a dispatch phase": {
			src:   `indri.before("received", function(req) end)`,
			wants: []string{"received", "only a built-in action can be hooked"},
		},
		"a lifecycle event": {
			src:   `indri.after_action("player:joined", function(req) end)`,
			wants: []string{"player:joined", "only a built-in action can be hooked"},
		},
		"an empty name": {
			src:   `indri.before("", function(req) end)`,
			wants: []string{"cannot be empty"},
		},
		"the same hook twice": {
			src: `
indri.before("join", function(req) end)
indri.before("join", function(req) end)
`,
			wants: []string{"already registered"},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := writeScript(t, "hooks.lua", test.src)

			engine, err := NewEngine([]string{path}, nil)
			if err == nil {
				engine.Close()
				t.Fatalf("the script loaded; want it refused because %v", test.wants)
			}

			requireErrorMentions(t, err, test.wants...)
		})
	}
}

// TestHooks_RefuseTheSameHookFromASecondScript covers the collision a single
// file cannot have: two scripts in one game, each hooking the same end of the
// same action. Both would run, in load order, and a game whose rule depends on
// which file was listed first in config.json is not a rule.
func TestHooks_RefuseTheSameHookFromASecondScript(t *testing.T) {
	t.Parallel()

	paths := writeScripts(t, map[string]string{
		"first.lua":  `indri.before("join", function(req) end)`,
		"second.lua": `indri.before("join", function(req) end)`,
	}, "first.lua", "second.lua")

	engine, err := NewEngine(paths, nil)
	if err == nil {
		engine.Close()
		t.Fatal("two scripts both hooked \"join\"; want the second refused")
	}

	requireErrorMentions(t, err, "already registered", "first.lua", "second.lua")
}

// TestHooks_BeforeAndAfterOnOneActionAreSeparate is the other half of the
// duplicate rule: the two kinds are two registrations, and neither shadows the
// other. Without this, the refusal above could be over-broad and nobody would
// notice until a game wanted both ends of the same action.
func TestHooks_BeforeAndAfterOnOneActionAreSeparate(t *testing.T) {
	t.Parallel()

	e := hookEngine(t, `
indri.before("join", function(req) indri.reply({at = "before"}) end)
indri.after_action("join", function(req) indri.reply({at = "after"}) end)
`)

	for kind, want := range map[HookKind]string{HookBefore: "before", HookAfterSuccess: "after"} {
		got := invokeHookReply(t, e, kind, "join", actions.Request{Session: &models.Session{}})
		if got["at"] != want {
			t.Errorf("the %s hook reported %v, want %q", kind, got["at"], want)
		}
	}
}

// TestHooks_RegistrationIsRefusedAfterLoading keeps a handler that kept hold of
// indri.before from adding a hook while the server is serving, which would
// mutate a map another invocation is reading.
func TestHooks_RegistrationIsRefusedAfterLoading(t *testing.T) {
	t.Parallel()

	e := hookEngine(t, `
local register = indri.before

indri.on("sneak", function(req)
  register("join", function(r) end)
end)
`)

	_, err := splitScriptError(e.Invoke(context.Background(), "sneak", actions.Request{}))
	requireErrorMentions(t, err, "while a script is loading")

	if got := e.Hooks(); len(got) != 0 {
		t.Errorf("the engine now reports the hooks %v, want the late registration refused", got)
	}
}

// --- a hook is not an action ------------------------------------------------

// TestHooks_AreNotActions is the containment this whole design rests on.
//
// Everything the framework builds an inbound surface from is Actions(): the
// router registrations, the REST route table, the GraphQL mutations, the
// scheduler's dispatchable set. A hook that leaked into it would hand a script
// a dispatchable handler named after a built-in — running *alongside* the real
// one, because registration is additive — which is precisely what
// validateRegistration refuses indri.on. Asserting the manifest alone would not
// be enough, so the dispatch path is driven too.
func TestHooks_AreNotActions(t *testing.T) {
	t.Parallel()

	e := hookEngine(t, `
indri.before("join", function(req) indri.reply({}) end)
indri.after_action("kick", function(req) indri.reply({}) end)
`)

	if got := e.Actions(); len(got) != 0 {
		t.Errorf("Actions() = %v, want no action: hooking a built-in must not declare one", got)
	}

	if got := e.Lifecycle(); len(got) != 0 {
		t.Errorf("Lifecycle() = %v, want no event", got)
	}

	for _, action := range []string{"join", "kick"} {
		_, err := e.Invoke(context.Background(), action, actions.Request{})
		requireErrorMentions(t, err, action, "no lua handler")
	}
}

// TestHooks_IndriOnStillRefusesABuiltin guards the door the hooks were added
// beside. indri.before deliberately names a built-in; indri.on must still not
// be able to.
func TestHooks_IndriOnStillRefusesABuiltin(t *testing.T) {
	t.Parallel()

	path := writeScript(t, "claim.lua", `indri.on("join", function(req) end)`)

	engine, err := NewEngine([]string{path}, nil)
	if err == nil {
		engine.Close()
		t.Fatal("a script registered a handler under the built-in action \"join\"")
	}

	requireErrorMentions(t, err, "join", "reserved")
}

// TestHooks_ManifestIsSortedAndCopied keeps Hooks() honest for its one caller:
// boot reads it once to build the handler's action set, so an unstable order
// would make the registry differ between boots, and a shared slice would let a
// caller edit what every state is held to.
func TestHooks_ManifestIsSortedAndCopied(t *testing.T) {
	t.Parallel()

	e := hookEngine(t, `
indri.after_action("leave", function(req) end)
indri.before("refresh", function(req) end)
indri.before("join", function(req) end)
`)

	want := []Hook{
		{Kind: HookAfterSuccess, Action: "leave"},
		{Kind: HookBefore, Action: "join"},
		{Kind: HookBefore, Action: "refresh"},
	}

	got := e.Hooks()
	if !slices.Equal(got, want) {
		t.Fatalf("Hooks() = %v, want %v", got, want)
	}

	got[0] = Hook{Kind: HookBefore, Action: "tampered"}

	if again := e.Hooks(); !slices.Equal(again, want) {
		t.Errorf("editing the returned slice changed the manifest to %v", again)
	}
}

// TestHookManifest_HoldsEveryStateToTheFirst covers the machinery that makes a
// hook the same rule for every player.
//
// States are built outside the pool lock, on whichever goroutine needed one, so
// a manifest that disagreed between states would mean a hook that fired for the
// players whose message happened to land on a state that had it. The engine
// path cannot produce a disagreement to test against — that is the point — so
// the manifest is driven directly.
func TestHookManifest_HoldsEveryStateToTheFirst(t *testing.T) {
	t.Parallel()

	first := []Hook{{Kind: HookBefore, Action: "join"}}

	var m hookManifest

	if err := m.agree(first); err != nil {
		t.Fatalf("the first state was refused: %v", err)
	}

	if err := m.agree([]Hook{{Kind: HookBefore, Action: "join"}}); err != nil {
		t.Errorf("a state registering the same hooks was refused: %v", err)
	}

	for name, got := range map[string][]Hook{
		"one hook more":      {{Kind: HookBefore, Action: "join"}, {Kind: HookBefore, Action: "leave"}},
		"one hook fewer":     {},
		"a different kind":   {{Kind: HookAfterSuccess, Action: "join"}},
		"a different action": {{Kind: HookBefore, Action: "leave"}},
	} {
		if err := m.agree(got); err == nil {
			t.Errorf("a state registering %s (%v) was accepted", name, got)
		}
	}

	if !slices.Equal(m.list(), first) {
		t.Errorf("the manifest is now %v, want the first state's %v", m.list(), first)
	}
}

// TestHooks_EveryPooledStateRegisteredThem is the manifest's claim exercised
// against real states.
//
// The pool builds a state per concurrent invocation, so running more hooks at
// once than the pool holds idle forces states that were never the boot-time one
// to serve. A hook missing from any of them would surface here as a host error
// saying no handler is registered — the failure a manifest agreed to by nobody
// would hide.
func TestHooks_EveryPooledStateRegisteredThem(t *testing.T) {
	t.Parallel()

	e := hookEngine(t, `indri.before("join", function(req) indri.reply({ok = true}) end)`)

	const callers = defaultMaxIdleStates * 4

	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		failed []error
	)

	for range callers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			res, err := e.InvokeHook(context.Background(), HookBefore, "join", actions.Request{
				Session: &models.Session{},
			})

			if err == nil && len(res.Responses) != 1 {
				err = fmt.Errorf("the hook produced %d responses, want one", len(res.Responses))
			}

			if err != nil {
				mu.Lock()
				failed = append(failed, err)
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	if len(failed) > 0 {
		t.Fatalf("%d of %d concurrent hook invocations failed, first: %v", len(failed), callers, failed[0])
	}
}

// --- what a hook is handed --------------------------------------------------

// TestInvokeHook_HandsTheActionAndPayloadToTheHook is the contract a hook is
// written against: it is one closure per action, but it still has to be able to
// say which action ran and what the caller asked for.
func TestInvokeHook_HandsTheActionAndPayloadToTheHook(t *testing.T) {
	t.Parallel()

	e := hookEngine(t, `
local function report(req)
  indri.reply({
    action = req.action,
    team = tostring(req.payload.team),
    userId = tostring(req.session.userId),
    gameId = tostring(req.gameId),
  })
end

indri.before("join", report)
indri.after_action("leave", report)
`)

	session := &models.Session{
		Token:  "the-bearer-token",
		UserID: stringPtr("player-1"),
		GameID: stringPtr("game-1"),
	}

	tests := map[string]struct {
		kind   HookKind
		action string
	}{
		"before":        {kind: HookBefore, action: "join"},
		"after-success": {kind: HookAfterSuccess, action: "leave"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := invokeHookReply(t, e, test.kind, test.action, actions.Request{
				Session: session,
				Payload: map[string]interface{}{"team": "spectators"},
			})

			want := map[string]interface{}{
				"action": test.action,
				"team":   "spectators",
				"userId": "player-1",
				"gameId": "game-1",
			}

			for key, value := range want {
				if got[key] != value {
					t.Errorf("the hook saw %s = %v, want %v", key, got[key], value)
				}
			}
		})
	}
}

// TestInvokeHook_StripsThePayloadForAnUnauthenticatedCaller is the second line
// behind refusing the credential actions.
//
// received runs before any Go handler has decided whether the caller may act,
// so an anonymous socket sending {"action":"join", ...} reaches a before hook
// with fields of its choosing. The hookable actions are not credential actions,
// so this is defence in depth rather than the rule itself — but a game's rules
// are about players it can name, and a hook with no caller to name has nothing
// to learn from their arguments.
func TestInvokeHook_StripsThePayloadForAnUnauthenticatedCaller(t *testing.T) {
	t.Parallel()

	e := hookEngine(t, `
indri.before("join", function(req)
  indri.reply({
    team = tostring(req.payload.team),
    session = tostring(req.session),
  })
end)
`)

	payload := map[string]interface{}{"team": "spectators"}

	tests := map[string]struct {
		session  *models.Session
		wantTeam string
	}{
		"authenticated": {session: &models.Session{UserID: stringPtr("player-1")}, wantTeam: "spectators"},
		"anonymous":     {session: nil, wantTeam: "nil"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := invokeHookReply(t, e, HookBefore, "join", actions.Request{
				Session: test.session,
				Payload: payload,
			})

			if got["team"] != test.wantTeam {
				t.Errorf("the hook saw payload.team = %v, want %q", got["team"], test.wantTeam)
			}
		})
	}

	// The caller's own Request must survive the stripping: the Go handler behind
	// the hook is given the same payload map, and a hook that emptied it would
	// break join rather than protect it.
	if payload["team"] != "spectators" {
		t.Errorf("the caller's payload is now %v; stripping reached the dispatcher's own copy", payload)
	}
}

// TestInvokeHook_LooksUpOneKindAndOneAction keeps the hook registry keyed on
// both halves.
//
// The negative half is what makes it worth writing. A registry keyed on the
// action alone would answer "join" with the before hook whichever kind was
// asked for, and every assertion that a hook "ran" would still pass.
func TestInvokeHook_LooksUpOneKindAndOneAction(t *testing.T) {
	t.Parallel()

	e := hookEngine(t, `indri.before("join", function(req) indri.reply({ran = true}) end)`)

	if got := invokeHookReply(t, e, HookBefore, "join", actions.Request{}); got["ran"] != true {
		t.Fatalf("the registered hook did not run: %v", got)
	}

	for name, lookup := range map[string]Hook{
		"the other kind on the hooked action": {Kind: HookAfterSuccess, Action: "join"},
		"the same kind on another action":     {Kind: HookBefore, Action: "leave"},
		"neither":                             {Kind: HookAfterSuccess, Action: "kick"},
	} {
		_, err := e.InvokeHook(context.Background(), lookup.Kind, lookup.Action, actions.Request{})
		if err == nil {
			t.Errorf("%s (%v) resolved to a hook", name, lookup)

			continue
		}

		requireErrorMentions(t, err, "no lua handler")
	}
}

// --- a hook that fails ------------------------------------------------------

// TestInvokeHook_FailureIsBothAFrameAndAnError is what makes indri.before able
// to refuse a move.
//
// Two channels, and both are load-bearing. The Go error is what aborts the
// phase chain, because router.Dispatch returns on the first non-nil error — a
// hook that only produced a frame would refuse a move that then went ahead
// anyway. The frame is what the player is actually told, because
// boot.handleClientMessage only log.Printfs a dispatch error and a WebSocket
// player would otherwise see nothing at all.
func TestInvokeHook_FailureIsBothAFrameAndAnError(t *testing.T) {
	t.Parallel()

	e := hookEngine(t, `indri.before("join", function(req) error("spectators are closed", 0) end)`)

	res, err := e.InvokeHook(context.Background(), HookBefore, "join", actions.Request{
		Session: &models.Session{},
	})

	if err == nil {
		t.Fatal("the hook raised but InvokeHook returned no error, so the phase chain would not abort")
	}

	if !strings.Contains(err.Error(), "spectators are closed") {
		t.Errorf("the error %q does not carry the hook's own words", err)
	}

	if !strings.Contains(err.Error(), `the before hook on the action "join"`) {
		t.Errorf("the error %q does not name the hook that raised", err)
	}

	if len(res.Responses) != 1 {
		t.Fatalf("the failure produced %d frames, want the one the player is told", len(res.Responses))
	}

	var frame models.WSError
	if err := json.Unmarshal(res.Responses[0], &frame); err != nil {
		t.Fatalf("the frame %q is not a models.WSError: %v", res.Responses[0], err)
	}

	if frame.ErrorCode != models.ErrScriptFailed.ErrorCode {
		t.Errorf("the frame carries code %d, want a script failure (%d)",
			frame.ErrorCode, models.ErrScriptFailed.ErrorCode)
	}

	if !strings.Contains(frame.Message, "spectators are closed") {
		t.Errorf("the frame %q does not carry the hook's own words", frame.Message)
	}
}

// --- which view a hook runs under -------------------------------------------

// hookViewsScript hooks both ends of one action and reports, from inside each
// hook, which capabilities are on the table it is running under.
//
// Both halves are asserted at each end, and the pair is what makes the test
// able to fail in both directions. "deferred is absent" alone would pass just as
// well if hooks were never bound to their script's views at all and were running
// on the bare shared table; "probe is present" is what rules that out.
const hookViewsScript = `
local function report(at)
  return function(req)
    assert(indri.deferred == nil, at .. ": indri.deferred is on the hook's view")
    assert(type(indri.probe) == "table", at .. ": indri.probe is missing from the hook's view")

    indri.probe.mark("hook:" .. at)
  end
end

indri.before("join", report("before"))
indri.after_action("join", report("after"))

indri.on("player:joined", function(ev)
  indri.deferred.mark("lifecycle")
end)
`

// TestHooks_RunOnTheActionView is the security half of the hook design under
// the two-view model.
//
// A hook runs inside the dispatch of the action it wraps, on that caller's
// budget, with a player waiting — so it gets the same narrowed view a dispatched
// action gets. Handing it views.full would put a capability like http back on
// the request path through the one mechanism built to extend that path, which is
// precisely what the views exist to stop.
//
// The lifecycle handler at the end is the control: the same script, the same
// grant list, and the capability that is absent from its hooks is reachable
// there. Without it this test would pass on an engine that had simply stopped
// granting the script anything.
func TestHooks_RunOnTheActionView(t *testing.T) {
	t.Parallel()

	calls := &probeLog{}
	path := writeScript(t, "hookviews.lua", hookViewsScript)

	e := newProbeEngine(t, calls, granted(path, probeName, deferredName))

	for _, kind := range hookKinds {
		if _, err := splitScriptError(
			e.InvokeHook(context.Background(), kind, "join", actions.Request{Session: &models.Session{}}),
		); err != nil {
			t.Fatalf("the %s hook failed: %v", kind, err)
		}
	}

	e.EmitLifecycle(LifecyclePlayerJoined, "game-1", nil)

	want := []string{"hook:before", "hook:after", "lifecycle"}

	if got := calls.all(); !slices.Equal(got, want) {
		t.Fatalf("the probes recorded %v, want %v", got, want)
	}
}

// TestHooks_OffTheActionViewAreUnreachableFromAHook is absence taken as
// seriously for a hook as for an action: the capability is not merely missing
// from indri, it cannot be found by walking the globals, the module tables or a
// metatable either.
//
// It reuses the walk reachScript performs for actions, rewritten to register as
// a hook. A view a capability was left off has to mean for a hook what it means
// for an action, or the hook is the way around it.
func TestHooks_OffTheActionViewAreUnreachableFromAHook(t *testing.T) {
	t.Parallel()

	calls := &probeLog{}

	// reachScript registers with indri.on; the same body has to run as a hook,
	// so the registration line is swapped and the rest is left exactly as the
	// action walk has it.
	walk := strings.Replace(
		reachScript("join", deferredName),
		`indri.on("join", function(req)`,
		`indri.before("join", function(req)`,
		1,
	)

	if strings.Contains(walk, `indri.on(`) {
		t.Fatal("the walk still registers with indri.on; it is not running as a hook")
	}

	path := writeScript(t, "reach.lua", walk)

	e := newProbeEngine(t, calls, granted(path, deferredName))

	if _, err := splitScriptError(
		e.InvokeHook(context.Background(), HookBefore, "join", actions.Request{}),
	); err != nil {
		t.Fatalf("the hook reached the capability: %v", err)
	}

	if got := calls.all(); len(got) != 0 {
		t.Fatalf("the probe recorded %v, want the capability never to have been reached", got)
	}
}

// --- a hook that writes -----------------------------------------------------

// TestHooks_MutateThroughIndriMutate proves a hook reaches game state the same
// way every other script handler does, and no other way.
//
// The store is the real one, so the write goes through the same lock, the same
// version fence and the same delta computation a player's move does — which is
// what "like anything else" has to mean. The published delta is asserted
// because a write nobody hears about is invisible to players.
func TestHooks_MutateThroughIndriMutate(t *testing.T) {
	store, publisher, id := newTestGame(t)

	e := newTestEngineWithGames(t, store, writeScript(t, "hooks.lua", `
indri.after_action("join", function(req)
  indri.mutate(function(state)
    state.data.lastJoin = req.session.userId

    return state
  end)
end)
`))

	if _, err := e.InvokeHook(context.Background(), HookAfterSuccess, "join", actions.Request{
		Session: &models.Session{UserID: stringPtr("player-1"), GameID: &id},
	}); err != nil {
		t.Fatalf("the hook failed: %v", err)
	}

	stored, err := store.Get(id)
	if err != nil {
		t.Fatalf("reading the game back: %v", err)
	}

	if got := stored.PublicData["lastJoin"]; got != "player-1" {
		t.Errorf("game.data.lastJoin = %v, want the hook's write", got)
	}

	if got := publisher.updatedPaths(); !slices.Contains(got, "data.lastJoin") {
		t.Errorf("the hook's write published %v, want it to name data.lastJoin", got)
	}
}

// TestHooks_WithoutAGameCannotMutate keeps a hook to the authority its caller
// already had.
//
// indri.mutate resolves the game from the invocation, which is built from the
// session the transport authenticated: a hook names no game and cannot. A
// caller who is in no game therefore gives a hook nothing to write to, which is
// what stops a hook on create or join — both of which run before their caller
// has a game — from reaching one of somebody else's.
func TestHooks_WithoutAGameCannotMutate(t *testing.T) {
	store, publisher, _ := newTestGame(t)

	e := newTestEngineWithGames(t, store, writeScript(t, "hooks.lua", `
indri.before("join", function(req)
  indri.mutate(function(state) state.data.round = 99; return state end)
end)
`))

	_, err := e.InvokeHook(context.Background(), HookBefore, "join", actions.Request{
		Session: &models.Session{UserID: stringPtr("player-1")},
	})
	requireErrorMentions(t, err, "not in a game")

	if got := publisher.count(); got != 0 {
		t.Errorf("the hook published %d deltas without a game of its own", got)
	}
}
