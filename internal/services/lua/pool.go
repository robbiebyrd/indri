package lua

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	lua "github.com/yuin/gopher-lua"
)

// readOnlyMarker is the metatable field freezeTable stamps on a table it has
// made read-only. It is invisible from Lua — the sandbox strips getmetatable —
// so only Go can read it.
const readOnlyMarker = "__indriReadOnly"

// errPoolClosed is returned by acquire after close.
var errPoolClosed = errors.New("the lua state pool is closed")

// pooledState is one sandboxed LState, plus the flag that decides whether it
// may ever be used again.
type pooledState struct {
	L *lua.LState

	// spoiled marks a state whose interpreter was interrupted rather than
	// returning. See spoils.
	spoiled bool
}

// statePool hands out sandboxed states, one per invocation.
//
// An LState is not goroutine-safe and building one is expensive — the sandbox
// opens five libraries, rewrites two of them and freezes the rest — so states
// are reused. Reuse is also the source of every problem this file solves: a
// pooled state remembers the globals, the module tables and the interpreter
// stack of whatever ran on it last.
//
// Three separate mechanisms keep one invocation from seeing the one before it:
//
//   - freezeState makes the real globals and every module table read-only, so
//     nothing a script can reach is writable in the first place;
//   - freshEnv gives each invocation its own globals table, discarded when the
//     call returns, as the only surface a script's writes can land on;
//   - spoils closes a state whose stack was interrupted instead of pooling it,
//     because nothing about that state can be trusted afterwards.
//
// The pool is a mutex-guarded slice and not a sync.Pool: sync.Pool discards
// entries during GC with no hook to run first, and an LState dropped without
// Close never releases what it holds. Shutdown here is deterministic.
type statePool struct {
	mu     sync.Mutex
	idle   []*pooledState
	closed bool

	maxIdle int
	prepare func(*lua.LState) error
}

// newPool returns a pool that keeps up to maxIdle prepared states.
//
// prepare runs on every state as it is built, before the state is frozen, and
// is where a caller installs its host API and runs its script chunks. It may be
// nil. It must be safe to call from several goroutines at once — states are
// built outside the pool lock — and it must be deterministic, because every
// state is expected to end up identical.
//
// One state is built here rather than lazily so that a prepare that cannot
// succeed fails at boot instead of on the first request.
func newPool(maxIdle int, prepare func(*lua.LState) error) (*statePool, error) {
	if maxIdle < 1 {
		return nil, fmt.Errorf("a lua state pool needs room for at least one idle state, got %d", maxIdle)
	}

	p := &statePool{maxIdle: maxIdle, prepare: prepare}

	first, err := p.build()
	if err != nil {
		return nil, err
	}

	p.idle = append(p.idle, first)

	return p, nil
}

// run calls proto on a pooled state and hands the result to consume.
//
// consume runs while the state is still held, and may be nil when the result is
// not wanted. That is not a convenience: a lua.LValue belongs to the state that
// produced it, so reading one after the state has gone back to the pool would
// race the next invocation.
func (p *statePool) run(ctx context.Context, proto *lua.FunctionProto, consume func(lua.LValue) error) error {
	s, err := p.acquire()
	if err != nil {
		return err
	}

	defer p.release(s)

	ret, err := s.call(ctx, s.instantiate(proto))
	if err != nil {
		return err
	}

	if consume == nil {
		return nil
	}

	return consume(ret)
}

// acquire takes an idle state, or builds one when none is free.
//
// The pool does not cap how many states exist at once, only how many it keeps:
// blocking a player's action behind another game's script would be a worse
// failure than the memory a burst of states costs.
func (p *statePool) acquire() (*pooledState, error) {
	p.mu.Lock()

	if p.closed {
		p.mu.Unlock()

		return nil, errPoolClosed
	}

	if n := len(p.idle); n > 0 {
		s := p.idle[n-1]
		p.idle[n-1] = nil
		p.idle = p.idle[:n-1]

		p.mu.Unlock()

		return s, nil
	}

	p.mu.Unlock()

	// Built outside the lock: construction opens and freezes five libraries and
	// has no business blocking every other invocation.
	return p.build()
}

// release returns s to the pool, or closes it when it cannot be reused.
//
// A spoiled state is never pooled. Neither is a state released after close, or
// one that arrives when the pool is already holding maxIdle.
func (p *statePool) release(s *pooledState) {
	p.mu.Lock()

	if !s.spoiled && !p.closed && len(p.idle) < p.maxIdle {
		p.idle = append(p.idle, s)
		p.mu.Unlock()

		return
	}

	p.mu.Unlock()

	s.L.Close()
}

// close closes every idle state and refuses further acquisitions. States that
// are checked out at the time are closed by their own release.
func (p *statePool) close() {
	p.mu.Lock()
	idle := p.idle
	p.idle, p.closed = nil, true
	p.mu.Unlock()

	for _, s := range idle {
		s.L.Close()
	}
}

// build makes one sandboxed state, prepares it and freezes it.
//
// The order is the contract: prepare has to run before freezeState, because a
// frozen state refuses every write a host would want to make.
func (p *statePool) build() (*pooledState, error) {
	L, err := defaultState()
	if err != nil {
		return nil, err
	}

	if p.prepare != nil {
		if err := p.prepare(L); err != nil {
			L.Close()

			return nil, fmt.Errorf("preparing a lua state: %w", err)
		}
	}

	if err := freezeState(L); err != nil {
		L.Close()

		return nil, err
	}

	return &pooledState{L: L}, nil
}

// instantiate binds shared bytecode to this state.
//
// Every state instantiates its own function from the shared proto. The result
// must not be cached anywhere the pool can hand to another state: an LFunction
// closes over the environment and upvalues of the state that created it.
func (s *pooledState) instantiate(proto *lua.FunctionProto) *lua.LFunction {
	return s.L.NewFunctionFromProto(proto)
}

// call invokes fn with args under ctx and returns its first result.
//
// ctx must not be nil; it is what stops a runaway script, since the VM checks
// it between instructions. It is removed again afterwards so a state going back
// to the pool does not carry an expired deadline into the next invocation.
//
// fn is given a fresh environment before it runs, which is what makes a pooled
// state safe to reuse. Note that the Go-side SetFEnv is unaffected by the
// sandbox stripping the Lua-side setfenv/getfenv: the host installs the
// environment precisely because the script cannot.
func (s *pooledState) call(ctx context.Context, fn *lua.LFunction, args ...lua.LValue) (lua.LValue, error) {
	s.L.SetFEnv(fn, freshEnv(s.L))

	s.L.SetContext(ctx)
	defer s.L.RemoveContext()

	s.L.Push(fn)

	for _, arg := range args {
		s.L.Push(arg)
	}

	if err := s.L.PCall(len(args), 1, nil); err != nil {
		// Latched, never recomputed: a state that has been spoiled once cannot
		// become trustworthy again by surviving a later call.
		s.spoiled = s.spoiled || spoils(ctx, err)

		return nil, err
	}

	ret := s.L.Get(-1)
	s.L.Pop(1)

	return ret, nil
}

// freshEnv builds the per-invocation globals table.
//
// The table starts empty and its metatable's __index points at the state's real
// globals, so a script reads the sandbox's libraries exactly as it would
// normally while every global it *assigns* lands in a table that is thrown away
// when the invocation ends. OP_SETGLOBAL writes through the running function's
// environment, so this needs no cooperation from the script.
//
// It is only half of the isolation. Without freezeState a script would still
// reach shared state through a table it does not have to assign to, starting
// with _G.
func freshEnv(L *lua.LState) *lua.LTable {
	env := L.NewTable()
	mt := L.NewTable()

	mt.RawSetString("__index", L.Get(lua.GlobalsIndex))
	L.SetMetatable(env, mt)

	return env
}

// spoils reports whether a failed invocation leaves its state unusable.
//
// Three failures do. A deadline or cancellation stops the VM wherever it
// happened to be, so the call stack was never unwound by the code that built
// it. An ApiErrorPanic is a Go panic that crossed the VM, which says nothing
// about how much of the interpreter's own invariants survived. An exhausted
// stack or registry has grown to its configured maximum and would spend the
// rest of the state's life there.
//
// Everything else — error(), a failed assert, indexing a nil, a rejected
// string.rep — is an ordinary Lua error that PCall unwinds completely, and the
// state is as good as it was before the call. Discarding a state on every
// script bug would turn a typo in a game script into a state rebuild per
// message.
func spoils(ctx context.Context, err error) bool {
	if ctx.Err() != nil {
		return true
	}

	var apiErr *lua.ApiError
	if !errors.As(err, &apiErr) {
		return false
	}

	if apiErr.Type == lua.ApiErrorPanic {
		return true
	}

	// gopher-lua raises both exhaustion errors as ordinary run errors with no
	// distinguishing type, so the message is the only signal available. Only
	// the raised object is matched, never the stack trace appended to it. A
	// script can forge one of these by calling error("stack overflow") itself,
	// which costs it nothing but a rebuilt state.
	message := apiErr.Object.String()

	return strings.Contains(message, "stack overflow") || strings.Contains(message, "registry overflow")
}

// freezeState makes every table a pooled state shares between invocations
// read-only.
//
// A metatable alone does not do it. Lua consults __newindex only for a key the
// table does not already have, so bolting one onto a populated table stops
// string.newField = x and does nothing whatsoever about string.format = evil —
// which is the mutation that actually leaks to the next invocation. Each frozen
// table is therefore *emptied* into a hidden backing table that its metatable's
// __index points at. Reads are unchanged, every write now finds a missing key
// and lands on __newindex, and the table object keeps its identity, so the
// string metatable, package.loaded and every existing reference still resolve
// to the right thing.
//
// The globals table is frozen like any other. That is what closes _G.x = 1,
// _G.string = evil and package.loaded._G — the routes by which a script reaches
// shared state without ever assigning to a global.
//
// Three consequences a caller has to know:
//
//   - Anything the host installs must be installed first. L.SetGlobal on a
//     frozen state raises, and outside a PCall that is a panic. The pool freezes
//     as the last step of building a state, after prepare.
//   - require can no longer load a module for the first time, because it
//     records the result with SetField on package.loaded. A module has to be
//     required once while the state is being prepared; the cached lookup keeps
//     working for every invocation after that.
//   - pairs() over a frozen table yields nothing: the table itself is empty and
//     Lua 5.1 has no __pairs. Frozen tables are libraries, not data.
func freezeState(L *lua.LState) error {
	globals, ok := L.Get(lua.GlobalsIndex).(*lua.LTable)
	if !ok {
		return errors.New("the globals table is missing")
	}

	pkg, ok := globals.RawGetString("package").(*lua.LTable)
	if !ok {
		return fmt.Errorf("package table is missing after opening %q", lua.LoadLibName)
	}

	loaded, ok := pkg.RawGetString("loaded").(*lua.LTable)
	if !ok {
		return errors.New("package.loaded is missing")
	}

	strlib, ok := globals.RawGetString("string").(*lua.LTable)
	if !ok {
		return fmt.Errorf("string table is missing after opening %q", lua.StringLibName)
	}

	// Collected before anything is frozen, because freezing empties a table and
	// a reference read afterwards would come back nil.
	targets := []namedTable{{"_G", globals}, {"package", pkg}}
	targets = append(targets, childTables("package.", pkg)...)
	targets = append(targets, childTables("", loaded)...)

	// table.insert writes through the array part with a raw set, which no
	// metatable can see, so it is capped before the tables it could write to are
	// frozen.
	if err := guardTableInsert(L); err != nil {
		return err
	}

	frozen := make(map[*lua.LTable]struct{}, len(targets))

	for _, target := range targets {
		freezeTable(L, target.name, target.tbl, frozen)
	}

	rebindStringMetatable(L, strlib)

	return nil
}

// rebindStringMetatable gives string values a metatable of their own.
//
// OpenString makes the string *table* the metatable of every string value and
// stores __index inside it, pointing at itself. Lua fetches a metamethod with a
// raw get, so emptying that table to freeze it takes __index away with it and
// ("x"):rep(2) stops resolving at all.
//
// Leaving __index behind as a raw key would fix the lookup and reopen the hole
// this file exists to close: a key that is already present is a key __newindex
// never sees, so a script could repoint every string method at a function of
// its own and the next invocation on that state would call it. A separate
// metatable — unreachable from Lua, since the sandbox strips both getmetatable
// and setmetatable — keeps method syntax working with the string table still
// read-only.
func rebindStringMetatable(L *lua.LState, strlib *lua.LTable) {
	mt := L.NewTable()
	mt.RawSetString("__index", strlib)

	L.SetMetatable(lua.LString(""), mt)
}

// namedTable pairs a table with the name a script would reach it by, which is
// what a refused write reports.
type namedTable struct {
	name string
	tbl  *lua.LTable
}

// childTables lists the table-valued entries of parent, so freezing package
// also freezes package.loaded, package.preload and package.loaders, and
// freezing package.loaded also freezes every module in it.
func childTables(prefix string, parent *lua.LTable) []namedTable {
	var children []namedTable

	parent.ForEach(func(k, v lua.LValue) {
		if tbl, ok := v.(*lua.LTable); ok {
			children = append(children, namedTable{prefix + k.String(), tbl})
		}
	})

	return children
}

// freezeTable empties tbl into a backing table reached through __index, and
// refuses every write with __newindex.
//
// The same table arrives under more than one name — package.loaded holds the
// globals table as _G and every library the globals table already holds — so
// frozen records what has been done and the first name wins.
//
// An existing metatable is extended rather than replaced, but its __index and
// __newindex are overwritten. Nothing in the sandbox has either; a host that
// adds one must do so knowing the freeze owns those two fields.
func freezeTable(L *lua.LState, name string, tbl *lua.LTable, frozen map[*lua.LTable]struct{}) {
	if _, done := frozen[tbl]; done {
		return
	}

	frozen[tbl] = struct{}{}

	backing := L.NewTable()

	var keys []lua.LValue

	tbl.ForEach(func(k, v lua.LValue) {
		backing.RawSet(k, v)
		keys = append(keys, k)
	})

	// Emptied after the walk, never during it: ForEach over a table being
	// mutated is not defined.
	for _, key := range keys {
		tbl.RawSet(key, lua.LNil)
	}

	mt, ok := L.GetMetatable(tbl).(*lua.LTable)
	if !ok {
		mt = L.NewTable()
		L.SetMetatable(tbl, mt)
	}

	mt.RawSetString("__index", backing)
	mt.RawSetString("__newindex", L.NewFunction(func(L *lua.LState) int {
		L.RaiseError("%s is read-only", name)

		return 0
	}))
	mt.RawSetString(readOnlyMarker, lua.LTrue)
}

// guardTableInsert stops table.insert from writing into a frozen table.
//
// insert appends through the array part with a raw set, so unlike an ordinary
// assignment it never consults __newindex: table.insert(string, "x") would
// plant a value on a shared library table, and the next invocation on that
// state would find it there. It is the only library function that can do this —
// remove and sort only touch entries a frozen, and therefore empty, table does
// not have.
func guardTableInsert(L *lua.LState) error {
	tbllib, ok := L.GetGlobal("table").(*lua.LTable)
	if !ok {
		return fmt.Errorf("table table is missing after opening %q", lua.TabLibName)
	}

	insert := tbllib.RawGetString("insert")

	tbllib.RawSetString("insert", L.NewFunction(func(L *lua.LState) int {
		if isReadOnly(L, L.CheckTable(1)) {
			L.RaiseError("cannot insert into a read-only table")
		}

		return delegate(L, insert, 0)
	}))

	return nil
}

// isReadOnly reports whether tbl has been through freezeTable.
func isReadOnly(L *lua.LState, tbl *lua.LTable) bool {
	mt, ok := L.GetMetatable(tbl).(*lua.LTable)

	return ok && mt.RawGetString(readOnlyMarker) == lua.LTrue
}
