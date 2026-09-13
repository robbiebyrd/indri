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

// capabilitySet is every capability this server recognises, by the name a grant
// list uses.
//
// A nil installer means the name is reserved and understood but not built yet.
// That is deliberately not the same as an unknown name: granting it is still a
// boot failure, but one that says the server cannot do this rather than that
// the operator misspelled something.
type capabilitySet map[string]capabilityInstaller

// defaultCapabilities is the set a real engine is built against.
//
// Both installers are configured here and nowhere else. What bounds a fetch and
// which directory an asset comes from are properties of the server, not of the
// script that was granted them, so a grant list decides only whether a script
// may reach the capability at all.
var defaultCapabilities = capabilitySet{
	CapabilityHTTP:   httpCapability(defaultHTTPConfig()),
	CapabilityAssets: assetsCapability(defaultAssetsConfig()),
}

// grantedCapability is one capability a particular script may reach, resolved
// from its name at boot so no lookup happens while a script is loading.
type grantedCapability struct {
	name    string
	install capabilityInstaller
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
		install, known := c[name]

		switch {
		case !known:
			return nil, fmt.Errorf(
				"the lua script %q is granted the unknown capability %q; this server knows %v",
				path, name, c.names(),
			)
		case install == nil:
			return nil, fmt.Errorf(
				"the lua script %q is granted the capability %q, which this server does not implement yet",
				path, name,
			)
		}

		if _, dup := seen[name]; dup {
			return nil, fmt.Errorf("the lua script %q is granted the capability %q more than once", path, name)
		}

		seen[name] = struct{}{}
		resolved = append(resolved, grantedCapability{name: name, install: install})
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

	actions, err := collectActions(chunks)
	if err != nil {
		return nil, err
	}

	e := &Engine{chunks: chunks, actions: actions, games: games}

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

// loadChunk runs one script with its own view of the host table.
//
// This is where "absent means unreachable" is built. A capability is never put
// on the shared indri table, so no script can find another script's grant by
// reading the global one, by walking _G — which is frozen and empty — or by
// following a metatable, because the only reference to a scoped table is held
// by Go: the closure this function binds around the handlers that script
// registered.
//
// The global indri is swapped for the scoped table while the chunk runs and put
// back afterwards, so a granted capability is reachable under the same name at
// load time and at run time, and is gone again before the next chunk starts.
//
// A script with no grants keeps the shared table. That is not only an
// optimisation: it means the common case adds no wrapper and no second table,
// so what a pooled state does for an ungranted script is exactly what it did
// before capabilities existed.
func loadChunk(L *lua.LState, h *stateHandlers, shared *lua.LTable, chunk scriptChunk) error {
	scoped, err := scopeHostTable(L, shared, chunk.caps)
	if err != nil {
		return fmt.Errorf("granting capabilities to the lua script %s: %w", chunk.name, err)
	}

	if scoped != shared {
		L.SetGlobal(hostTableName, scoped)
		defer L.SetGlobal(hostTableName, shared)
	}

	// Taken before the chunk runs, so what it registers can be told apart from
	// what the scripts before it did.
	before := h.names()

	L.Push(L.NewFunctionFromProto(chunk.proto))

	if err := L.PCall(0, 0, nil); err != nil {
		return fmt.Errorf("loading lua script %s: %w", chunk.name, err)
	}

	if scoped != shared {
		freezeScope(L, scoped)
		bindScope(L, h, scoped, before)
	}

	return nil
}

// scopeHostTable builds one script's own view of the indri table: everything
// the host installed for every script, plus the capabilities this one was
// granted.
//
// The shared table is copied rather than chained behind a metatable because the
// scoped table is frozen afterwards, and freezing owns __index — see
// freezeTable. A copy is safe here because the host API is complete before any
// chunk runs.
func scopeHostTable(L *lua.LState, shared *lua.LTable, caps []grantedCapability) (*lua.LTable, error) {
	if len(caps) == 0 {
		return shared, nil
	}

	scoped := L.NewTable()

	shared.ForEach(func(k, v lua.LValue) {
		scoped.RawSet(k, v)
	})

	for _, capability := range caps {
		if existing := scoped.RawGetString(capability.name); existing != lua.LNil {
			return nil, fmt.Errorf(
				"the capability %q would shadow the host function %s.%s",
				capability.name, hostTableName, capability.name,
			)
		}

		value, err := capability.install(L)
		if err != nil {
			return nil, fmt.Errorf("installing the %q capability: %w", capability.name, err)
		}

		scoped.RawSetString(capability.name, value)
	}

	return scoped, nil
}

// freezeScope makes a scoped host table and every capability table on it
// read-only.
//
// A scoped table outlives the invocation that reads it — it belongs to the
// state, not to the call — so without this one invocation could blank a
// capability, or repoint one of its functions, for every invocation after it on
// that state.
func freezeScope(L *lua.LState, scoped *lua.LTable) {
	frozen := make(map[*lua.LTable]struct{})

	// Collected and frozen before the parent, because freezing empties a table
	// and a child read afterwards would come back nil.
	for _, child := range childTables(hostTableName+".", scoped) {
		freezeTable(L, child.name, child.tbl, frozen)
	}

	freezeTable(L, hostTableName, scoped, frozen)
}

// bindScope replaces every handler the chunk just registered with one that runs
// under that script's own host table.
//
// It is needed because a handler's environment is not the one its chunk was
// loaded with: pooledState.call installs a fresh environment on every
// invocation, whose fallback is the state's real globals and therefore the
// *shared* indri table. Without this, a granted capability would be reachable
// while the script loaded and nil by the time a player triggered it.
//
// before is the manifest as it stood when the chunk started, so a handler
// another script registered is left alone.
func bindScope(L *lua.LState, h *stateHandlers, scoped *lua.LTable, before []string) {
	for action, fn := range h.fns {
		if slices.Contains(before, action) {
			continue
		}

		h.fns[action] = scopedHandler(L, fn, scoped)
	}
}

// scopedHandler wraps one handler so that indri, for the length of the call,
// means the table its own script was given.
//
// The fresh environment is still built per invocation, exactly as
// pooledState.call would have built it, so a global the handler assigns is
// still thrown away when the call ends. The only difference is the one raw
// entry that shadows the shared host table.
func scopedHandler(L *lua.LState, fn *lua.LFunction, scoped *lua.LTable) *lua.LFunction {
	return L.NewFunction(func(L *lua.LState) int {
		env := freshEnv(L)
		env.RawSetString(hostTableName, scoped)

		L.SetFEnv(fn, env)

		return delegate(L, fn, 1)
	})
}
