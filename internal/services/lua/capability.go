package lua

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	lua "github.com/yuin/gopher-lua"

	"github.com/robbiebyrd/indri/internal/models"
)

// The capability names a script may ask for in its config.json grants list.
//
// A name exists here so that a typo is caught: an unrecognised grant fails boot
// rather than quietly granting nothing, which would leave a script that looks
// permitted unable to do the thing it was permitted to do.
const (
	CapabilityHTTP   = "http"
	CapabilityAssets = "assets"
)

// capabilityInstaller builds one capability's value on L.
//
// It runs while the state is still writable, once per script that was granted
// the capability and once per pooled state, so it must not capture anything
// belonging to another state. The value it returns is raw-set on that script's
// own host table and then frozen.
type capabilityInstaller func(L *lua.LState) (lua.LValue, error)

// capability is one capability this server recognises: how to build it, and
// which kinds of invocation may reach it.
type capability struct {
	// install builds the value. A nil installer means the name is reserved and
	// understood but not built yet. That is deliberately not the same as an
	// unknown name: granting it is still a boot failure, but one that says the
	// server cannot do this rather than that the operator misspelled something.
	install capabilityInstaller

	// inAction says whether the capability is on the host table a dispatched
	// action handler runs under. False means it is *absent* from that table —
	// not present and refusing — so what a handler may do stays a matter of what
	// it was handed rather than of what some function body checks.
	//
	// It is read once, while a script's views of the host table are built, and
	// never at call time. See scopeHostTable and trigger.hostView.
	inAction bool
}

// capabilitySet is every capability this server recognises, by the name a grant
// list uses.
type capabilitySet map[string]capability

// defaultCapabilities is the set a real engine is built against.
//
// Both installers are configured here and nowhere else. What bounds a fetch and
// which directory an asset comes from are properties of the server, not of the
// script that was granted them, so a grant list decides only whether a script
// may reach the capability at all.
var defaultCapabilities = capabilitySet{
	// Absent from a dispatched action, and that is the constraint the capability
	// lives under rather than a detail of it. The only way a script changes
	// anything from an action is indri.mutate, whose callback runs inside the
	// store's apply closure with the game's distributed lock held and is re-run
	// on every version-fence retry: a blocking fetch there would hold the lock
	// for the whole timeout and pay it again on each retry. http therefore
	// belongs to a lifecycle handler, which holds no lock and keeps nobody
	// waiting.
	CapabilityHTTP: {install: httpCapability(defaultHTTPConfig())},

	// On every view. An asset read is a bounded read of a file an operator put
	// in a directory on this host, so it costs a lock holder a disk read rather
	// than however long somebody else's server takes to answer.
	CapabilityAssets: {install: assetsCapability(defaultAssetsConfig()), inAction: true},
}

// grantedCapability is one capability a particular script may reach, resolved
// from its name at boot so no lookup happens while a script is loading.
type grantedCapability struct {
	capability

	name string
}

// names lists the recognised capability names, sorted, for an error to quote.
func (c capabilitySet) names() []string {
	return slices.Sorted(maps.Keys(c))
}

// resolve turns one script's grant list into the installers it earns.
//
// Every refusal names the script and the grant. A grant list is operator-typed
// configuration, and the failure modes it has — a misspelling, a capability
// this build does not carry, the same grant twice — are all invisible at run
// time if they are allowed to pass: the script simply finds nil where it
// expected a capability, on a player's first move rather than at boot.
func (c capabilitySet) resolve(path string, grants []string) ([]grantedCapability, error) {
	resolved := make([]grantedCapability, 0, len(grants))
	seen := make(map[string]struct{}, len(grants))

	for _, name := range grants {
		known, exists := c[name]

		switch {
		case !exists:
			return nil, fmt.Errorf(
				"the lua script %q is granted the unknown capability %q; this server knows %v",
				path, name, c.names(),
			)
		case known.install == nil:
			return nil, fmt.Errorf(
				"the lua script %q is granted the capability %q, which this server does not implement yet",
				path, name,
			)
		}

		if _, dup := seen[name]; dup {
			return nil, fmt.Errorf("the lua script %q is granted the capability %q more than once", path, name)
		}

		seen[name] = struct{}{}
		resolved = append(resolved, grantedCapability{capability: known, name: name})
	}

	return resolved, nil
}

// NewEngineWithGrants compiles the configured scripts and returns an engine
// whose every state has run them, each with the capabilities its config entry
// granted it and nothing else.
//
// This is the form boot uses. NewEngine is the same thing with no grants at
// all.
//
// The caller owns the engine and must Close it.
func NewEngineWithGrants(scripts []models.ScriptFile, games GameMutator) (*Engine, error) {
	return newEngine(scripts, games, defaultCapabilities)
}

// newEngine is the whole construction, with the capability set as a parameter
// so a test can prove injection and absence against a capability of its own
// rather than against whichever ones this build happens to ship.
//
// Order matters. The scripts are compiled and their grants resolved first, so a
// syntax error or a bad grant is reported before anything runs; the manifest is
// then collected on a throwaway state, so the set of actions is fixed before a
// single pooled state exists; only then is the pool built, and its first state
// is the first one measured against that manifest.
func newEngine(scripts []models.ScriptFile, games GameMutator, caps capabilitySet) (*Engine, error) {
	chunks, err := compileGranted(scripts, caps)
	if err != nil {
		return nil, err
	}

	actions, lifecycle, err := collectRegistrations(chunks)
	if err != nil {
		return nil, err
	}

	e := &Engine{chunks: chunks, actions: actions, lifecycle: lifecycle, games: games}

	pool, err := newPool(defaultMaxIdleStates, e.prepare)
	if err != nil {
		return nil, err
	}

	e.pool = pool

	return e, nil
}

// compileGranted compiles each configured script and attaches the capabilities
// it was granted to its chunk.
//
// The grants travel with the bytecode because every pooled state re-runs every
// chunk and has to hand each one the same capabilities the boot-time state saw.
func compileGranted(scripts []models.ScriptFile, caps capabilitySet) ([]scriptChunk, error) {
	paths := make([]string, 0, len(scripts))

	for i, script := range scripts {
		if strings.TrimSpace(script.Path) == "" {
			return nil, fmt.Errorf("lua script %d in the config has no path", i+1)
		}

		paths = append(paths, script.Path)
	}

	// compileScripts preserves order and refuses a repeated path, so chunk i is
	// script i.
	chunks, err := compileScripts(paths)
	if err != nil {
		return nil, err
	}

	for i := range chunks {
		granted, err := caps.resolve(scripts[i].Path, scripts[i].Grants)
		if err != nil {
			return nil, err
		}

		chunks[i].caps = granted
	}

	return chunks, nil
}

// ungranted pairs each path with an empty grant list.
func ungranted(paths []string) []models.ScriptFile {
	scripts := make([]models.ScriptFile, 0, len(paths))

	for _, path := range paths {
		scripts = append(scripts, models.ScriptFile{Path: path})
	}

	return scripts
}

// loadChunk runs one script with its own views of the host table.
//
// This is where "absent means unreachable" is built. A capability is never put
// on the shared indri table, so no script can find another script's grant by
// reading the global one, by walking _G — which is frozen and empty — or by
// following a metatable, because the only reference to a scoped table is held
// by Go: the closure this function binds around the handlers that script
// registered.
//
// The global indri is swapped for the script's full view while the chunk runs
// and put back afterwards, so a granted capability is reachable under the same
// name at load time and at run time, and is gone again before the next chunk
// starts. Load time gets the full view because a chunk is not a dispatched
// action: nobody is waiting on it and no game lock is held.
//
// A script with no grants keeps the shared table. That is not only an
// optimisation: it means the common case adds no wrapper and no second table,
// so what a pooled state does for an ungranted script is exactly what it did
// before capabilities existed.
func loadChunk(L *lua.LState, h *stateHandlers, shared *lua.LTable, chunk scriptChunk) error {
	views, err := scopeHostTable(L, shared, chunk.caps)
	if err != nil {
		return fmt.Errorf("granting capabilities to the lua script %s: %w", chunk.name, err)
	}

	scoped := views.full != shared

	if scoped {
		L.SetGlobal(hostTableName, views.full)
		defer L.SetGlobal(hostTableName, shared)
	}

	// Taken before the chunk runs, so what it registers can be told apart from
	// what the scripts before it did. Both namespaces, because a lifecycle
	// handler needs its script's capabilities exactly as an action handler does.
	before := h.registered()

	L.Push(L.NewFunctionFromProto(chunk.proto))

	if err := L.PCall(0, 0, nil); err != nil {
		return fmt.Errorf("loading lua script %s: %w", chunk.name, err)
	}

	if scoped {
		freezeScope(L, views)
		bindScope(L, h, views, before)
	}

	return nil
}

// hostViews is one script's indri table, once per kind of invocation.
//
// Two tables built at load time rather than one table edited per call. A scoped
// table belongs to the pooled state and outlives the invocation that reads it,
// so a view produced by putting a capability on and taking it off again would
// still be whatever the last call left it as — for the next call on that state,
// and for the rest of the state's life if the call that was to take it off again
// was interrupted instead of unwound. Both tables are built once and frozen, and
// choosing between them is a read.
type hostViews struct {
	// action is what a dispatched action handler sees: the shared host API plus
	// only those granted capabilities that may be reached from a player's
	// request path.
	action *lua.LTable

	// full is everything the script was granted. A lifecycle handler runs under
	// it, and so does the chunk itself while it loads.
	full *lua.LTable
}

// scopeHostTable builds one script's own views of the indri table: everything
// the host installed for every script, plus the capabilities this one was
// granted, on the views each of those capabilities belongs to.
//
// The shared table is copied rather than chained behind a metatable because a
// scoped table is frozen afterwards, and freezing owns __index — see
// freezeTable. A copy is safe here because the host API is complete before any
// chunk runs.
//
// Each capability is installed once and the same value is put on both views, so
// a script granted one never talks to two of them.
func scopeHostTable(L *lua.LState, shared *lua.LTable, caps []grantedCapability) (hostViews, error) {
	if len(caps) == 0 {
		return hostViews{action: shared, full: shared}, nil
	}

	full := copyTable(L, shared)

	// One table unless a grant is actually absent from the action view, so a
	// script granted nothing but action-safe capabilities costs what it did
	// before there were two views.
	action := full
	if slices.ContainsFunc(caps, func(c grantedCapability) bool { return !c.inAction }) {
		action = copyTable(L, shared)
	}

	for _, capability := range caps {
		if existing := shared.RawGetString(capability.name); existing != lua.LNil {
			return hostViews{}, fmt.Errorf(
				"the capability %q would shadow the host function %s.%s",
				capability.name, hostTableName, capability.name,
			)
		}

		value, err := capability.install(L)
		if err != nil {
			return hostViews{}, fmt.Errorf("installing the %q capability: %w", capability.name, err)
		}

		full.RawSetString(capability.name, value)

		if capability.inAction && action != full {
			action.RawSetString(capability.name, value)
		}
	}

	return hostViews{action: action, full: full}, nil
}

// copyTable returns a new table holding everything src holds.
func copyTable(L *lua.LState, src *lua.LTable) *lua.LTable {
	dst := L.NewTable()

	src.ForEach(func(k, v lua.LValue) {
		dst.RawSet(k, v)
	})

	return dst
}

// freezeScope makes both of a script's views, and every capability table on
// them, read-only.
//
// A scoped table outlives the invocation that reads it — it belongs to the
// state, not to the call — so without this one invocation could blank a
// capability, or repoint one of its functions, for every invocation after it on
// that state.
//
// One record of what has been frozen covers both views, because a capability on
// both of them is the same table object and freezing empties what it freezes: a
// second pass over an already-frozen table would back up the empty husk and
// leave the capability with nothing on it.
func freezeScope(L *lua.LState, views hostViews) {
	frozen := make(map[*lua.LTable]struct{})

	for _, view := range []*lua.LTable{views.full, views.action} {
		// Collected and frozen before the parent, because freezing empties a
		// table and a child read afterwards would come back nil.
		for _, child := range childTables(hostTableName+".", view) {
			freezeTable(L, child.name, child.tbl, frozen)
		}

		freezeTable(L, hostTableName, view, frozen)
	}
}

// bindScope replaces every handler the chunk just registered with one that runs
// under that script's own view of the host table.
//
// It is needed because a handler's environment is not the one its chunk was
// loaded with: pooledState.call installs a fresh environment on every
// invocation, whose fallback is the state's real globals and therefore the
// *shared* indri table. Without this, a granted capability would be reachable
// while the script loaded and nil by the time a player triggered it.
//
// before is the manifest as it stood when the chunk started, so a handler
// another script registered is left alone.
func bindScope(L *lua.LState, h *stateHandlers, views hostViews, before []string) {
	for _, name := range h.registered() {
		if slices.Contains(before, name) {
			continue
		}

		fn, ok := h.lookup(name)
		if !ok {
			fn, ok = h.lookupLifecycle(name)
		}

		if !ok {
			continue
		}

		h.rebind(name, scopedHandler(L, fn, views))
	}
}

// scopedHandler wraps one handler so that indri, for the length of the call,
// means the view of its own script's table that this kind of call runs under.
//
// The fresh environment is still built per invocation, exactly as
// pooledState.call would have built it, so a global the handler assigns is
// still thrown away when the call ends. The only difference is the one raw
// entry that shadows the shared host table.
func scopedHandler(L *lua.LState, fn *lua.LFunction, views hostViews) *lua.LFunction {
	return L.NewFunction(func(L *lua.LState) int {
		env := freshEnv(L)
		env.RawSetString(hostTableName, hostViewFor(L, views))

		L.SetFEnv(fn, env)

		return delegate(L, fn, 1)
	})
}

// hostViewFor is the view the call now in progress runs under.
//
// The trigger decides, because why a handler is running is what governs what it
// may reach: a capability the action view does not carry is not present and
// refusing there, it is not there at all. See trigger.hostView.
//
// A call with no invocation on the state gets the action view. Nothing in the
// engine runs a registered handler without installing one, so this is the answer
// to a question that should not be asked, and the narrower view is the right
// answer to give it.
func hostViewFor(L *lua.LState, views hostViews) *lua.LTable {
	inv, err := currentInvocation(L)
	if err != nil || inv.trigger == nil {
		return views.action
	}

	return inv.trigger.hostView(views)
}
