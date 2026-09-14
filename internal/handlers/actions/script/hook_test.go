package script

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
	"github.com/robbiebyrd/indri/internal/injector"
	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/services/lua"
)

// recordingHookEngine stands in for the Lua engine and keeps everything it was
// asked to run, so a test can assert on what the handler decided rather than on
// what a script happened to do.
//
// result and err are what it answers with, which is how a test drives a return
// value the real engine cannot produce today — the point of the session
// assertion below.
type recordingHookEngine struct {
	ctx    context.Context
	kind   lua.HookKind
	action string
	req    actions.Request

	result actions.Result
	err    error
}

func (e *recordingHookEngine) InvokeHook(
	ctx context.Context,
	kind lua.HookKind,
	action string,
	req actions.Request,
) (actions.Result, error) {
	e.ctx, e.kind, e.action, e.req = ctx, kind, action, req

	return e.result, e.err
}

// TestHookHandler_RunsOnlyTheActionsItWasGiven is the per-action rule, and the
// reason a global phase is safe to register under.
//
// router.Dispatch runs the received and processed phases for *every* inbound
// message, so this handler is invoked on all of them. What makes a hook
// per-action is the set consulted here and nothing else — get it wrong and a
// hook on "join" fires on every move, every refresh and every login in the game.
//
// The handler is built with no injector at all, which turns "reached the
// engine" into an observable error rather than something a test has to infer.
// An assertion that a hooked action ran would otherwise pass just as happily
// with the filter deleted.
func TestHookHandler_RunsOnlyTheActionsItWasGiven(t *testing.T) {
	handler := NewHook(nil, lua.HookBefore, []string{"join", "leave"})

	t.Run("a hooked action reaches the engine", func(t *testing.T) {
		for _, action := range []string{"join", "leave"} {
			_, err := handler.Handle(actions.Request{Action: action})
			if err == nil {
				t.Errorf("the hook on %q did not reach the engine", action)

				continue
			}

			if !strings.Contains(err.Error(), action) {
				t.Errorf("the error %q does not name the action that could not run", err)
			}
		}
	})

	t.Run("every other action is a no-op", func(t *testing.T) {
		for name, action := range map[string]string{
			"another hookable built-in":          "kick",
			"a credential action":                "login",
			"a script's own action":              "move",
			"the phase this is registered under": "received",
			"no action at all":                   "",
		} {
			res, err := handler.Handle(actions.Request{Action: action})
			if err != nil {
				t.Errorf("%s (%q) reached the engine: %v", name, action, err)
			}

			if len(res.Responses) != 0 || len(res.DisconnectIDs) != 0 {
				t.Errorf("%s (%q) produced %v", name, action, res)
			}
		}
	})
}

// TestHookHandler_WithoutAnEngineReportsIt covers the injector the registry
// tests build. Reaching a hooked action with no engine loaded is a wiring
// failure and must read as one rather than panic in a nil dereference.
func TestHookHandler_WithoutAnEngineReportsIt(t *testing.T) {
	tests := map[string]*injector.Injector{
		"no injector":             nil,
		"no services":             {},
		"services with no engine": {ServicesInjector: &injector.ServicesInjector{}},
	}

	for name, i := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := NewHook(i, lua.HookAfterSuccess, []string{"join"}).
				Handle(actions.Request{Action: "join"})
			if err == nil {
				t.Fatal("a hook answered although no engine is loaded")
			}

			for _, want := range []string{"join", string(lua.HookAfterSuccess)} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("the error %q does not name %q", err, want)
				}
			}
		})
	}
}

// TestInvokeHook_StripsTheSessionFromTheResult is the identity boundary.
//
// router.Dispatch adopts any non-nil Result.Session as the session for the
// remaining phases and hands it back to the transport, which binds the
// connection to it — that is how login establishes auth. A hook runs on a
// session the transport already authenticated, so one able to return a
// different session could promote its own caller to anybody.
//
// The Lua engine cannot produce that field today, which is exactly why this is
// tested against a fake that does: a test driven through the real engine would
// pass with the stripping deleted and go on passing until the day something
// started setting it.
func TestInvokeHook_StripsTheSessionFromTheResult(t *testing.T) {
	admin := &models.Session{UserID: stringPtr("the-host")}

	tests := map[string]error{
		"a hook that succeeded": nil,
		"a hook that failed":    errors.New("the hook raised"),
	}

	for name, hookErr := range tests {
		t.Run(name, func(t *testing.T) {
			engine := &recordingHookEngine{
				result: actions.Result{
					Session:       admin,
					Responses:     [][]byte{[]byte(`{"reply":true}`)},
					DisconnectIDs: []string{"session-1"},
				},
				err: hookErr,
			}

			res, err := invokeHook(
				actions.Request{Action: "join"}, engine, lua.HookBefore, InvocationTimeout)

			if !errors.Is(err, hookErr) {
				t.Fatalf("invokeHook returned %v, want %v", err, hookErr)
			}

			if res.Session != nil {
				t.Errorf("the hook handed back the session %v; a hook cannot re-authenticate its caller", res.Session)
			}

			// The other two halves of the Result are a hook's legitimate output —
			// indri.reply writes Responses and indri.send("kick") writes
			// DisconnectIDs — so stripping must not be a blanket reset.
			if len(res.Responses) != 1 || string(res.Responses[0]) != `{"reply":true}` {
				t.Errorf("the hook's responses were lost: %q", res.Responses)
			}

			if len(res.DisconnectIDs) != 1 || res.DisconnectIDs[0] != "session-1" {
				t.Errorf("the hook's disconnects were lost: %v", res.DisconnectIDs)
			}
		})
	}
}

// TestInvokeHook_HandsTheEngineTheKindAndTheDispatchedAction keeps the two
// halves of a hook lookup straight.
//
// The kind is fixed at registration, so a payload cannot redirect a before hook
// into the after-success registry. The action comes from the dispatch, because
// one handler covers every action of its kind — reading it from anywhere else
// would run the wrong hook.
func TestInvokeHook_HandsTheEngineTheKindAndTheDispatchedAction(t *testing.T) {
	engine := &recordingHookEngine{}

	session := &models.Session{UserID: stringPtr("player-1")}
	req := actions.Request{
		Action:  "join",
		Session: session,
		Payload: map[string]interface{}{"team": "reds"},
	}

	if _, err := invokeHook(req, engine, lua.HookAfterSuccess, InvocationTimeout); err != nil {
		t.Fatalf("invokeHook: %v", err)
	}

	if engine.kind != lua.HookAfterSuccess {
		t.Errorf("the engine was asked for the %q hook, want the registered %q", engine.kind, lua.HookAfterSuccess)
	}

	if engine.action != "join" {
		t.Errorf("the engine was asked for the hook on %q, want the dispatched %q", engine.action, "join")
	}

	if engine.req.Session != session {
		t.Errorf("the engine got session %v, want the transport's %v", engine.req.Session, session)
	}

	if engine.req.Payload["team"] != "reds" {
		t.Errorf("the engine got payload %v, want the caller's %v", engine.req.Payload, req.Payload)
	}
}

// TestInvokeHook_SharesTheCallersBudget is the reason a hook cannot extend an
// action's deadline.
//
// A hook runs inside the same dispatch as the action it wraps, so deriving its
// context from the caller's is what keeps a client that has gone away — or a
// shutdown — from leaving a hook running for the full timeout, and what stops a
// hook plus an action costing twice a script action's budget.
func TestInvokeHook_SharesTheCallersBudget(t *testing.T) {
	t.Run("a caller whose budget is already shorter", func(t *testing.T) {
		caller := deadlineIn(t, InvocationTimeout/4)
		engine := &recordingHookEngine{}

		if _, err := invokeHook(
			actions.Request{Context: caller, Action: "join"}, engine, lua.HookBefore, InvocationTimeout,
		); err != nil {
			t.Fatalf("invokeHook: %v", err)
		}

		want, _ := caller.Deadline()

		got, ok := engine.ctx.Deadline()
		if !ok {
			t.Fatal("the engine was invoked with no deadline, so a runaway hook would never be stopped")
		}

		if !got.Equal(want) {
			t.Errorf("the hook was given a deadline %v past its caller's", got.Sub(want))
		}
	})

	t.Run("a caller with no budget of its own", func(t *testing.T) {
		engine := &recordingHookEngine{}
		before := time.Now()

		if _, err := invokeHook(
			actions.Request{Action: "join"}, engine, lua.HookBefore, InvocationTimeout,
		); err != nil {
			t.Fatalf("invokeHook: %v", err)
		}

		after := time.Now()

		deadline, ok := engine.ctx.Deadline()
		if !ok {
			t.Fatal("the engine was invoked with no deadline at all")
		}

		if got := deadline.Sub(after); got > InvocationTimeout {
			t.Errorf("the hook was given %v to run in, want no more than %v", got, InvocationTimeout)
		}

		if got := deadline.Sub(before); got < InvocationTimeout {
			t.Errorf("the hook was given %v to run in, want at least %v", got, InvocationTimeout)
		}
	})
}

func stringPtr(s string) *string {
	return &s
}
