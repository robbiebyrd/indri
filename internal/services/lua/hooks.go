package lua

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	lua "github.com/yuin/gopher-lua"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
)

// HookKind is when a hook runs relative to the Go handler it wraps.
//
// The two names are the honest ones. "before" is unqualified because a before
// hook really does run on every dispatch of its action. Its partner is
// after-*success* and not "after", because router.Dispatch returns the moment a
// handler errors (internal/handlers/router/act.go): a failed action never
// reaches the processed phase, so there is no such thing as an after hook that
// runs whatever happened. Calling one "after" would promise a cleanup hook the
// dispatcher cannot deliver.
//
// # A before hook should validate and not write
//
// The two are not equally safe places to change state, and the asymmetry has no
// fix at this layer. A before hook's indri.mutate commits on its own — the
// store's lock and version fence cover that one write and nothing around it —
// so if the built-in action behind it then fails, the hook's write stands
// against a move that never happened. There is no transaction spanning the two:
// games are stored in a standalone mongod (CLAUDE.md), so no multi-document
// transaction exists to enrol them in, and the phases are separate dispatches
// besides.
//
// So a before hook is for refusing a move — raise, and router.Dispatch stops
// the chain with nothing written — and an after-success hook is where a state
// change belongs, because by then the action it is reacting to has committed.
type HookKind string

const (
	HookBefore       HookKind = "before"
	HookAfterSuccess HookKind = "after-success"
)

// hookKinds is every kind a script may register, in the order they run.
var hookKinds = []HookKind{HookBefore, HookAfterSuccess}

// hostHookNames is the indri function each kind is registered through.
//
// indri.after is already the timer (host_timer.go), so the after-success hook
// cannot be called that; after_action says what it wraps and leaves the timer
// its name. What it does *not* say is that it only runs on success — that is
// what the doc comments, the kind constant and the error messages are for.
var hostHookNames = map[HookKind]string{
	HookBefore:       "before",
	HookAfterSuccess: "after_action",
}

// authActions are the built-ins no script may hook, at either end.
//
// login's payload carries a plaintext password (internal/handlers/actions/login)
// and register's carries the one a player is about to be given. The whole point
// of running game logic behind the Lua sandbox is that credentials never sit on
// the script side of that boundary, and a hook is the one mechanism that
// deliberately reaches a built-in, so this is where that promise is kept.
// reconnect and logout are here for the same reason at one remove: reconnect's
// payload is the bearer token a client resumes with, and logout is the action
// that invalidates it.
var authActions = []string{"login", "logout", "reconnect", "register"}

// hookableActions is every built-in a script may wrap.
//
// Derived from builtinActions rather than listed again, so a built-in added to
// the framework becomes hookable by default and *not* hooking it is the change
// somebody has to argue for. The subtraction is the whole security rule in one
// line, and TestHookableActions_ExcludesEveryAuthAction holds it.
func hookableActions() []string {
	hookable := make([]string, 0, len(builtinActions))

	for _, action := range builtinActions {
		if !slices.Contains(authActions, action) {
			hookable = append(hookable, action)
		}
	}

	return hookable
}

// Hook names one registration: a kind, and the built-in action it wraps.
type Hook struct {
	Kind   HookKind
	Action string
}

func (h Hook) describe() string {
	return fmt.Sprintf("the %s hook on the action %q", h.Kind, h.Action)
}

// hostName is the indri call a script registered this hook with, for an error
// that has to name the line the author wrote.
func (h Hook) hostName() string {
	return hostTableName + "." + hostHookNames[h.Kind]
}

// stateHooks is one state's hook registrations.
//
// A third registry beside stateHandlers.fns and .lifecycle, and deliberately
// not an entry in either. Everything the framework builds an inbound surface
// from — the router registrations in boot.registerScriptHandlers, the REST
// route table, the GraphQL mutations, the scheduler's dispatchable set — is
// built from Engine.Actions(), which is stateHandlers.fns and nothing else. A
// hook that lived in that map would therefore hand a script a *dispatchable*
// action named after a built-in, which is the exact thing reservedReason
// refuses. Keeping hooks in a map no dispatch can reach makes "a hook cannot
// become a replacement handler" true by construction rather than by a check
// somebody has to remember.
//
// It is not guarded by a mutex for the same reason stateHandlers is not: it is
// written only while its own state is being prepared, on the goroutine doing
// the preparing, and a *lua.LState is single-threaded by construction.
type stateHooks struct {
	fns map[Hook]*lua.LFunction

	// sources records the file and line each hook was registered from, so a
	// duplicate can name both halves of the collision. Keyed on the Hook rather
	// than on the action, because before and after-success on the same action
	// are two different registrations and neither shadows the other.
	sources map[Hook]string
}

func newStateHooks() *stateHooks {
	return &stateHooks{
		fns:     make(map[Hook]*lua.LFunction),
		sources: make(map[Hook]string),
	}
}

// lookup returns the closure registered for hook on this state.
func (s *stateHooks) lookup(hook Hook) (*lua.LFunction, bool) {
	fn, ok := s.fns[hook]

	return fn, ok
}

// names lists what this state hooked, in a stable order, so two states'
// registrations can be compared as they are.
func (s *stateHooks) names() []Hook {
	hooks := make([]Hook, 0, len(s.fns))

	for hook := range s.fns {
		hooks = append(hooks, hook)
	}

	slices.SortFunc(hooks, func(a, b Hook) int {
		if a.Kind != b.Kind {
			return strings.Compare(string(a.Kind), string(b.Kind))
		}

		return strings.Compare(a.Action, b.Action)
	})

	return hooks
}

// hookRegistrar builds indri.before or indri.after_action for one state.
//
// The closure it stores is the script's own, unwrapped. A hook registered by a
// script that was granted a capability is rebound afterwards by bindHooks, for
// the reason every other handler is — see there.
//
// Every refusal is an L.RaiseError rather than a returned error, exactly as
// stateHandlers.register's are: it surfaces as an ordinary Lua error prefixed
// with the offending script's file and line and unwinds the PCall the chunk is
// running under, so the load fails. A script that cannot hook what it asked for
// must not start — a hook that registers successfully but never runs is
// indistinguishable from a game rule nobody implemented.
func hookRegistrar(h *stateHandlers, kind HookKind) lua.LGFunction {
	return func(L *lua.LState) int {
		action := L.CheckString(1)
		fn := L.CheckFunction(2)

		hook := Hook{Kind: kind, Action: action}

		if h.sealed {
			L.RaiseError("%s(%q): hooks can only be registered while a script is loading", hook.hostName(), action)
		}

		if reason := hookRefusal(action); reason != "" {
			L.RaiseError("%s: %s", hook.hostName(), reason)
		}

		if where, dup := h.hooks.sources[hook]; dup {
			L.RaiseError("%s: %s is already registered at %s", hook.hostName(), hook.describe(), where)
		}

		h.hooks.sources[hook] = strings.TrimSuffix(L.Where(1), ":")
		h.hooks.fns[hook] = fn

		return 0
	}
}

// bindHooks is bindScope for the third registry: it replaces every hook the
// chunk just registered with one that runs under that script's own views of the
// host table.
//
// It is needed for the reason bindScope is needed, and the reasoning is there
// rather than repeated here: a handler's environment is not the one its chunk
// was loaded with, so without this a granted capability would be reachable while
// the script loaded and nil by the time a player triggered the action it hooked.
//
// It is a second function rather than a case inside bindScope because the two
// registries are keyed differently — an action by name, a hook by kind *and*
// action — and because bindScope walks stateHandlers.registered(), which is
// deliberately the two *dispatchable* namespaces. Growing that to cover hooks
// would put them within reach of a dispatch, which is the one thing the third
// registry exists to prevent.
//
// before is the hook manifest as it stood when the chunk started, so a hook
// another script registered is left alone.
func bindHooks(L *lua.LState, h *stateHandlers, views hostViews, before []Hook) {
	for _, hook := range h.hooks.names() {
		if slices.Contains(before, hook) {
			continue
		}

		fn, ok := h.hooks.lookup(hook)
		if !ok {
			continue
		}

		h.hooks.fns[hook] = scopedHandler(L, fn, views)
	}
}

// hookRefusal explains why action is not a script's to hook, or returns an
// empty string when it is.
//
// The allow-list is positive on purpose. A hook names an action that already
// exists in Go, so "unknown" and "forbidden" are the same check, and a
// misspelled indri.before("jion", ...) is a boot failure rather than a rule the
// author will spend an afternoon wondering why nobody enforces.
func hookRefusal(action string) string {
	switch {
	case strings.TrimSpace(action) == "":
		return "an action name cannot be empty"
	case slices.Contains(authActions, action):
		return fmt.Sprintf(
			"the action %q handles credentials and cannot be hooked; a hook on it would be handed "+
				"the caller's password or bearer token",
			action,
		)
	case !slices.Contains(builtinActions, action):
		return fmt.Sprintf(
			"only a built-in action can be hooked, and %q is not one; this server's hookable actions are %v",
			action, hookableActions(),
		)
	default:
		return ""
	}
}

// hookManifest is the set of hooks every state is held to.
//
// It is collected here rather than by collectRegistrations, which the other two
// manifests come from, because collectRegistrations is called from newEngine
// and its signature is shared with work in flight. The semantics are the same
// either way: the first state to be built decides the manifest and every state
// after it must agree, and the first state is built synchronously inside
// newPool before newEngine returns, so a caller holding an *Engine is holding
// one whose manifest is already settled.
//
// The mutex is what makes that safe to say. Pooled states are built outside the
// pool lock, on whichever goroutine needed one (statePool.acquire), so prepare
// genuinely runs concurrently once the engine is serving.
type hookManifest struct {
	mu    sync.Mutex
	hooks []Hook
	set   bool
}

// agree records the first state's hooks and holds every state after it to them.
//
// A disagreement is a boot failure for the reason stateHandlers.agreesWith
// gives: registration is expected to be deterministic, and a hook that varied
// between states would fire on the players whose message happened to land on a
// state that had it and not on the rest — a game rule that applies at random.
func (m *hookManifest) agree(got []Hook) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.set {
		m.hooks, m.set = slices.Clone(got), true

		return nil
	}

	if slices.Equal(m.hooks, got) {
		return nil
	}

	return fmt.Errorf(
		"lua hook registration is not deterministic: this state registered the hooks %v, "+
			"but the manifest collected at boot says %v",
		got, m.hooks,
	)
}

// list returns a copy of the manifest.
func (m *hookManifest) list() []Hook {
	m.mu.Lock()
	defer m.mu.Unlock()

	return slices.Clone(m.hooks)
}

// Hooks returns the built-in actions the scripts wrapped, in a stable order.
//
// Deliberately separate from Actions(). Everything built from Actions() is an
// inbound surface — a router registration, a REST route, a GraphQL mutation —
// and a hook is not dispatchable by anybody: it is reached only through the
// received/processed phases the dispatcher already runs.
func (e *Engine) Hooks() []Hook {
	return e.hooks.list()
}

// InvokeHook runs one script hook around a built-in action.
//
// # Why the payload is dropped for an unauthenticated caller
//
// A hook runs in the received or processed phase, and those phases run before
// any Go handler has decided whether the caller may act at all — so an
// anonymous socket sending {"action":"join", ...} reaches a before hook with
// whatever fields it chose. Hooking the credential actions is already refused
// (hookRefusal), which is the rule that matters; this is the second line,
// covering the payload a client can attach to a *hookable* action while
// unauthenticated. A hook exists to extend a game's rules, and a game's rules
// are about players it can name, so a hook that has no caller to name has
// nothing to learn from their arguments either.
//
// The Request is taken by value, so clearing Payload here cannot reach the copy
// the dispatcher is still holding for the Go handler behind this hook.
func (e *Engine) InvokeHook(
	ctx context.Context,
	kind HookKind,
	action string,
	req actions.Request,
) (actions.Result, error) {
	if req.Session == nil {
		req.Payload = nil
	}

	t := hookTrigger{hook: Hook{Kind: kind, Action: action}}

	return e.run(ctx, t, req.Session, req.GameID(), func(L *lua.LState) (lua.LValue, error) {
		return requestToLua(L, action, req)
	})
}

// hookTrigger is a script hook wrapped around a built-in action.
type hookTrigger struct {
	hook Hook
}

func (t hookTrigger) describe() string {
	return t.hook.describe()
}

// lookup reaches the hook registry and only that. An action never resolves to a
// hook and a hook never resolves to an action, which is what keeps a script from
// reaching a built-in's name through the dispatcher — see stateHooks.
func (t hookTrigger) lookup(h *stateHandlers) (*lua.LFunction, bool) {
	return h.hooks.lookup(t.hook)
}

// failed answers a hook's own failure through *both* channels, which neither of
// the other two triggers does.
//
// The frame is owed for the reason actionTrigger owes one: there is a player
// waiting on the action this hook wrapped, and Responses is the one channel
// every transport writes back. The Go error is owed because a hook that raises
// has to abort the phase chain exactly as a Go handler's error does —
// router.Dispatch returns on the first non-nil error, which is what makes
// indri.before the place a game refuses a move, and what stops an after-success
// hook's failure from being silently swallowed.
//
// Both together are not a contradiction: Dispatch merges a handler's Responses
// before it inspects the error, so the caller gets the frame *and* the chain
// stops.
func (t hookTrigger) failed(err error) (actions.Result, error) {
	return scriptFailure(t.describe(), err), fmt.Errorf("%s raised: %w", t.describe(), err)
}

// hostView hands back the narrowed view — the same one a dispatched action runs
// under, and not the lifecycle handler's full one.
//
// A hook is part of the request path, whatever else it is. It runs inside the
// dispatch of the action it wraps, on that caller's own budget, with a player
// waiting on the answer: every reason actionTrigger is given the narrow view
// applies here unchanged. Handing a hook views.full would put http back on the
// request path through the one mechanism designed to extend it, which is
// exactly the route the two views exist to close.
//
// The kind makes no difference. An after-success hook runs after the action's
// handler returned, but still inside the same dispatch and still before the
// caller is answered, so it is no freer than a before hook. A script that wants
// the full view has a lifecycle handler, which runs on the queue's own
// goroutine with nobody waiting.
func (t hookTrigger) hostView(views hostViews) *lua.LTable {
	return views.action
}
