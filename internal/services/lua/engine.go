package lua

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// hostTableName is the single global a script reaches the host through.
const hostTableName = "indri"

// handlersRegistryKey is where a prepared state keeps its own handler map.
//
// The registry is a Go-side table: no library the sandbox opens exposes it, and
// debug — which is how Lua would otherwise reach it — is never opened. Keeping
// the map there rather than in a Go map keyed by *lua.LState ties its lifetime
// to the state's own, so a state closed by the pool takes its handlers with it.
const handlersRegistryKey = "indri.handlers"

// loadTimeout bounds the whole of one state's load: every chunk, start to
// finish. A chunk runs at boot and on every state the pool builds afterwards,
// so a script that loops at the top level would otherwise hang the server
// before it ever served a request.
const loadTimeout = 10 * time.Second

// defaultMaxIdleStates is how many prepared states the engine keeps for reuse.
const defaultMaxIdleStates = 8

// dispatchPhases are the two pseudo-actions router.Dispatch runs around every
// message. A handler registered under either name runs on *all* traffic, so a
// script must not be able to claim one by accident.
var dispatchPhases = []string{"received", "processed"}

// builtinActions is every action the framework registers itself.
//
// They are refused to scripts because registration is additive: a script
// claiming "login" would not replace the built-in handler, it would run
// alongside it, on a payload carrying a plaintext password. The list is
// checked against the action packages on disk by
// TestBuiltinActions_CoversEveryActionPackage.
var builtinActions = []string{
	"create",
	"inquire",
	"join",
	"kick",
	"layout",
	"leave",
	"login",
	"logout",
	"reconnect",
	"refresh",
	"register",
}

// Engine is the compiled, loaded set of game scripts.
//
// It holds two things and deliberately not a third. It holds the bytecode,
// which is immutable and safe to share, and the action manifest, which is
// collected once at boot and then frozen. It does *not* hold the registered
// handlers: indri.on takes an anonymous closure, a closure may capture
// upvalues, a *lua.FunctionProto cannot reconstruct upvalues and a
// *lua.LFunction belongs to the state that created it. There is therefore no
// such thing as "the" handler for an action — there is one per state, and each
// pooled state builds its own by running every chunk itself.
//
// The manifest is what makes that arrangement safe to route against: every
// state is checked against it as it is built, so a script whose registrations
// vary between states fails the build instead of serving an action on some
// connections and not others.
type Engine struct {
	chunks  []scriptChunk
	actions []string

	// lifecycle is the other half of the manifest: the game lifecycle events
	// the scripts subscribed to. It is kept apart from actions because the two
	// namespaces are reached from opposite directions — an action is dispatched
	// by the router, an event is raised by the server — and everything built
	// from Actions() (the router registrations, the REST route table, the
	// scheduler's dispatchable set) must never see one of these.
	lifecycle []string

	// events is the deferred queue every lifecycle event passes through. See
	// lifecycleQueue: nothing is dispatched inline by the call that raised it.
	events lifecycleQueue

	// games is what indri.mutate edits through. It is held here rather than
	// passed to Invoke because it is a property of the server, not of a
	// request: every invocation on every state edits the same store.
	games GameMutator
	pool  *statePool

	// Dispatch is how an event indri.send queued reaches the router once the
	// handler that queued it has returned. It is an exported field set at boot
	// rather than a constructor argument, the same shape as
	// rest.Transport.Dispatch and for the same reason: the router is a
	// package-level registry in the handler layer, and a service reaching back
	// into it would invert the dependency. It must be set before the engine
	// serves anything; a nil one makes indri.send refuse rather than drop.
	Dispatch Dispatcher

	// Timers is where indri.after, indri.at and indri.cancel write. Exported and
	// set at boot for the same reason Dispatch is: the schedule store is a repo,
	// and the interface here is deliberately narrow enough that an adapter over
	// it satisfies this without the lua package importing one. A nil one makes
	// all three refuse rather than silently lose a timer, which would hang a
	// game with nothing in the log.
	Timers GameScheduler
}

// scriptChunk is one script file's bytecode, under the path it was read from.
// The path is the chunk name the VM reports, which is what puts a file and a
// line into every syntax and runtime error.
type scriptChunk struct {
	name  string
	proto *lua.FunctionProto

	// caps is what this script's config entry granted it, resolved at boot. It
	// travels with the bytecode because every pooled state re-runs every chunk
	// and each one has to be handed the same capabilities. See capability.go.
	caps []grantedCapability
}

// NewEngine compiles the scripts at paths and returns an engine whose every
// state has run them, granting none of them a capability. A boot driven by
// config.json wants NewEngineWithGrants.
//
// An empty paths list is not an error. It yields an engine that declares no
// actions.
//
// games is what indri.mutate writes through. A nil games is allowed and yields
// an engine whose scripts load and run but whose indri.mutate raises — which is
// what a test that only exercises registration or marshalling wants, and is a
// clear error rather than a nil dereference if it ever reaches production.
//
// The caller owns the engine and must Close it.
func NewEngine(paths []string, games GameMutator) (*Engine, error) {
	return NewEngineWithGrants(ungranted(paths), games)
}

// Actions returns the declared action names, sorted. The caller gets a copy:
// the manifest is frozen, and the router is not allowed to disagree with it.
//
// Lifecycle events are deliberately not here. An action is dispatchable — by a
// client, by a timer, over three transports — and an event is not.
func (e *Engine) Actions() []string {
	return slices.Clone(e.actions)
}

// Lifecycle returns the lifecycle events some script subscribed to, sorted.
func (e *Engine) Lifecycle() []string {
	return slices.Clone(e.lifecycle)
}

// Close releases every state the engine is holding.
func (e *Engine) Close() {
	if e.pool != nil {
		e.pool.close()
	}
}

// prepare is the pool's per-state hook: it runs every chunk on the new state
// and refuses the state if what it registered differs from the manifest.
//
// It runs before the state is frozen, which is the only time the host can
// install a global and the only time require can load a module for the first
// time.
func (e *Engine) prepare(L *lua.LState) error {
	h, err := installScripts(L, e.chunks)
	if err != nil {
		return err
	}

	return h.agreesWith(e.actions, e.lifecycle)
}

// compileScripts reads and compiles each path, in the order given.
func compileScripts(paths []string) ([]scriptChunk, error) {
	cache := newProtoCache()
	chunks := make([]scriptChunk, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))

	for _, path := range paths {
		if _, dup := seen[path]; dup {
			return nil, fmt.Errorf("lua script %q is listed more than once", path)
		}

		seen[path] = struct{}{}

		src, err := os.ReadFile(filepath.Clean(path))
		if err != nil {
			return nil, fmt.Errorf("reading lua script %q: %w", path, err)
		}

		// The path is the chunk name, so the compile error already reads
		// "path:line:column: message" and needs no decoration here.
		proto, err := cache.compile(path, string(src))
		if err != nil {
			return nil, err
		}

		chunks = append(chunks, scriptChunk{name: path, proto: proto})
	}

	return chunks, nil
}

// collectRegistrations loads the chunks on a state built for the purpose and
// returns both halves of what they registered: the actions, and the lifecycle
// events they subscribed to.
//
// The state is closed again immediately. Nothing it produced can outlive it —
// the handlers it built are bound to it — so the names are all that is kept,
// and they are what every other state is then held to.
func collectRegistrations(chunks []scriptChunk) (actions, lifecycle []string, err error) {
	L, err := defaultState()
	if err != nil {
		return nil, nil, err
	}

	defer L.Close()

	h, err := installScripts(L, chunks)
	if err != nil {
		return nil, nil, err
	}

	return h.names(), h.lifecycleNames(), nil
}

// installScripts installs the host API on L, runs every chunk, and leaves the
// resulting handler map on the state.
//
// It must be called before the state is frozen. Three things here need a
// writable state: the indri global, the module cache behind any require a chunk
// performs at load time, and any global a chunk assigns to.
//
// A chunk runs with the state's own globals as its environment, not a
// per-invocation one. That is deliberate: whatever a chunk leaves in the
// globals is frozen with them, so it is visible to every later invocation and
// writable by none of them.
func installScripts(L *lua.LState, chunks []scriptChunk) (*stateHandlers, error) {
	h := newStateHandlers()

	if err := installHostAPI(L, h); err != nil {
		return nil, err
	}

	// The deadline covers the loop rather than one chunk, and is removed again
	// so a state going on to the pool carries no expired context.
	ctx, cancel := context.WithTimeout(context.Background(), loadTimeout)
	defer cancel()

	L.SetContext(ctx)
	defer L.RemoveContext()

	shared, err := hostTable(L)
	if err != nil {
		return nil, err
	}

	// Each chunk runs under its own view of the host table, so a capability one
	// script was granted is not on the table the next one sees. See loadChunk.
	for _, chunk := range chunks {
		if err := loadChunk(L, h, shared, chunk); err != nil {
			return nil, err
		}
	}

	// Registration is a load-time act. Sealing before the host API is frozen
	// means a handler that somehow kept hold of indri.on can still call it and
	// still be refused, rather than mutating a map another invocation is
	// reading.
	h.seal()

	freezeHostTable(L)

	ud := L.NewUserData()
	ud.Value = h

	L.G.Registry.RawSetString(handlersRegistryKey, ud)

	return h, nil
}

// installHostAPI puts indri.on on the state's host table.
//
// Every other host function belongs on the same table and must be installed
// here too, before the chunks run: a chunk may call one at load time, and
// nothing can be added to the table once it is frozen.
func installHostAPI(L *lua.LState, h *stateHandlers) error {
	indri, err := hostTable(L)
	if err != nil {
		return err
	}

	indri.RawSetString("on", L.NewFunction(h.register))

	// Installed once per state and shared by every invocation that runs on it.
	// It reads the caller's game, deadline and store from the state's
	// invocation registry rather than capturing them here — see invocation.
	indri.RawSetString("mutate", L.NewFunction(hostMutate))

	// The two ways a script speaks to the world outside its own game document.
	// Neither acts when it is called: both queue on the invocation's ledger and
	// are released only once the handler has returned and the write it belonged
	// to has committed. See host_io.go.
	indri.RawSetString("reply", L.NewFunction(hostReply))
	indri.RawSetString("send", L.NewFunction(hostSend))

	// The deferred half of the same idea: an action dispatched at a time rather
	// than now. These queue too, so a timer set inside a mutate that lost its
	// version fence is discarded with the attempt. See host_timer.go.
	indri.RawSetString("after", L.NewFunction(hostAfter))
	indri.RawSetString("at", L.NewFunction(hostAt))
	indri.RawSetString("cancel", L.NewFunction(hostCancel))

	return nil
}

// hostTable returns the state's indri table, creating it if it is not there
// yet.
func hostTable(L *lua.LState) (*lua.LTable, error) {
	switch existing := L.GetGlobal(hostTableName).(type) {
	case *lua.LTable:
		return existing, nil
	case *lua.LNilType:
		indri := L.NewTable()
		L.SetGlobal(hostTableName, indri)

		return indri, nil
	default:
		return nil, fmt.Errorf("the global %q is already a %s", hostTableName, existing.Type())
	}
}

// freezeHostTable makes the host table read-only, for the same reason
// freezeState makes the libraries read-only: the table is shared by every
// invocation that runs on this state, so one invocation assigning indri.on = nil
// would otherwise break the next.
//
// It is frozen on its own rather than by freezeState, which only reaches the
// globals table and the module tables. The globals table is frozen separately
// and later, so the two never meet the same table twice.
func freezeHostTable(L *lua.LState) {
	indri, ok := L.GetGlobal(hostTableName).(*lua.LTable)
	if !ok {
		return
	}

	freezeTable(L, hostTableName, indri, map[*lua.LTable]struct{}{})
}

// stateHandlers is one state's registrations: the closures it built and where
// each was registered from.
//
// It is not guarded by a mutex and does not need one. It is written only while
// its own state is being prepared, on the goroutine doing the preparing, and a
// *lua.LState is single-threaded by construction.
type stateHandlers struct {
	fns map[string]*lua.LFunction

	// lifecycle holds the subscriptions, in a map of their own rather than
	// alongside the actions. Two registries is what makes "an action never
	// resolves to a lifecycle handler" true by construction instead of by a
	// check somebody has to remember: the lookup a dispatch performs cannot
	// reach this map at all.
	lifecycle map[string]*lua.LFunction

	// sources records the file and line each name was registered from, so a
	// duplicate can name both halves of the collision. One map covers both
	// registries because the two namespaces cannot collide — a lifecycle name
	// carries a colon and an action name may not.
	sources map[string]string

	// sealed closes registration once loading is over.
	sealed bool
}

func newStateHandlers() *stateHandlers {
	return &stateHandlers{
		fns:       make(map[string]*lua.LFunction),
		lifecycle: make(map[string]*lua.LFunction),
		sources:   make(map[string]string),
	}
}

// register is indri.on(name, fn), for both an action and a lifecycle event.
//
// One host function rather than two because a script author registers a handler
// either way, and the name says which registry it lands in. What that does not
// mean is one namespace: the name is validated against the rules of whichever
// kind it claims to be, and lands in that kind's own map.
//
// Every refusal here is an L.RaiseError rather than a returned error, so it
// surfaces as an ordinary Lua error prefixed with the offending script's file
// and line, and unwinds the PCall that installScripts is running the chunk
// under. The load then fails, which is the point: a script that cannot register
// what it asked for must not start. That is what makes a misspelled lifecycle
// event a boot failure rather than a handler that quietly never runs.
func (h *stateHandlers) register(L *lua.LState) int {
	name := L.CheckString(1)
	fn := L.CheckFunction(2)

	if h.sealed {
		L.RaiseError("indri.on(%q): handlers can only be registered while a script is loading", name)
	}

	if err := validateRegistration(name); err != nil {
		L.RaiseError("indri.on: %s", err.Error())
	}

	if where, dup := h.sources[name]; dup {
		L.RaiseError("indri.on: %q is already registered at %s", name, where)
	}

	h.sources[name] = strings.TrimSuffix(L.Where(1), ":")

	if isLifecycleName(name) {
		h.lifecycle[name] = fn

		return 0
	}

	h.fns[name] = fn

	return 0
}

// seal ends load time. Registration is refused from here on.
func (h *stateHandlers) seal() {
	h.sealed = true
}

// names lists the registered actions, sorted, so two states' registrations can
// be compared as they are.
func (h *stateHandlers) names() []string {
	return slices.Sorted(maps.Keys(h.fns))
}

// lifecycleNames lists the subscribed lifecycle events, sorted.
func (h *stateHandlers) lifecycleNames() []string {
	return slices.Sorted(maps.Keys(h.lifecycle))
}

// registered lists every name this state bound, across both registries. It is
// what tells a handler a chunk has just registered from one an earlier chunk
// did — see bindScope.
func (h *stateHandlers) registered() []string {
	return append(h.names(), h.lifecycleNames()...)
}

// lookup returns the closure registered for action on this state.
func (h *stateHandlers) lookup(action string) (*lua.LFunction, bool) {
	fn, ok := h.fns[action]

	return fn, ok
}

// lookupLifecycle returns the closure subscribed to event on this state.
func (h *stateHandlers) lookupLifecycle(event string) (*lua.LFunction, bool) {
	fn, ok := h.lifecycle[event]

	return fn, ok
}

// rebind replaces the closure registered under name, in whichever registry
// holds it. Used by bindScope, which wraps a handler in its script's own host
// table after the chunk that registered it has run.
func (h *stateHandlers) rebind(name string, fn *lua.LFunction) {
	if isLifecycleName(name) {
		h.lifecycle[name] = fn

		return
	}

	h.fns[name] = fn
}

// agreesWith reports whether this state registered exactly the manifest, in
// both namespaces.
//
// A disagreement is a boot failure, not something to paper over. Registration
// is expected to be deterministic; a script that derives an action name from
// anything that varies between states — a table address through tostring, a
// random number — would otherwise give every pooled state a different action
// set, and which actions a player could reach would depend on which state their
// message happened to land on. A lifecycle subscription that varied would be
// worse, because nothing dispatches it: the event would simply not be told to
// some of the states, and a game would behave differently depending on which
// one the emission landed on.
func (h *stateHandlers) agreesWith(actions, lifecycle []string) error {
	for _, manifest := range []struct {
		kind string
		got  []string
		want []string
	}{
		{kind: "actions", got: h.names(), want: actions},
		{kind: "lifecycle events", got: h.lifecycleNames(), want: lifecycle},
	} {
		if slices.Equal(manifest.got, manifest.want) {
			continue
		}

		return fmt.Errorf(
			"lua script registration is not deterministic: this state registered the %s %v, "+
				"but the manifest collected at boot says %v",
			manifest.kind, manifest.got, manifest.want,
		)
	}

	return nil
}

// handlersFor returns the handler map a prepared state owns.
func handlersFor(L *lua.LState) (*stateHandlers, error) {
	ud, ok := L.G.Registry.RawGetString(handlersRegistryKey).(*lua.LUserData)
	if !ok {
		return nil, errors.New("this lua state has not loaded any scripts")
	}

	h, ok := ud.Value.(*stateHandlers)
	if !ok {
		return nil, fmt.Errorf("the lua handler registry holds a %T", ud.Value)
	}

	return h, nil
}

// isLifecycleName reports whether a registered name claims to be a lifecycle
// event rather than an action. It says nothing about whether it is one this
// server raises — validateRegistration decides that.
func isLifecycleName(name string) bool {
	return strings.Contains(name, lifecycleMark)
}

// validateRegistration refuses a name a script may not register under.
//
// The two namespaces have different rules and each name is held to the rules of
// the one it claims. A name carrying the lifecycle mark must be an event this
// server raises; anything else is an action, and may not be a dispatch phase or
// a built-in.
func validateRegistration(name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("a name cannot be empty")
	}

	if isLifecycleName(name) {
		if reason := lifecycleReason(name); reason != "" {
			return errors.New(reason)
		}

		return nil
	}

	if reason := reservedReason(name); reason != "" {
		return fmt.Errorf("the action name %q is reserved: %s", name, reason)
	}

	return nil
}

// reservedReason explains why action is not a script's to take, or returns an
// empty string when it is.
func reservedReason(action string) string {
	switch {
	case slices.Contains(dispatchPhases, action):
		return "it is a dispatch phase that runs on every message"
	case slices.Contains(builtinActions, action):
		return "it is a built-in framework action"
	default:
		return ""
	}
}
