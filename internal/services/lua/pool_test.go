package lua

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// newTestPool builds a pool and closes it when the test ends.
func newTestPool(t *testing.T, maxIdle int, prepare func(*lua.LState) error) *statePool {
	t.Helper()

	p, err := newPool(maxIdle, prepare)
	if err != nil {
		t.Fatalf("building a state pool: %v", err)
	}

	t.Cleanup(p.close)

	return p
}

// idleCount reports how many states the pool is holding for reuse.
func idleCount(p *statePool) int {
	p.mu.Lock()
	defer p.mu.Unlock()

	return len(p.idle)
}

// runProto runs proto on a pooled state and returns its result as a Go value.
//
// The conversion happens inside the consume callback on purpose: that is the
// only window in which the result's state is still held.
func runProto(t *testing.T, p *statePool, proto *lua.FunctionProto) (any, error) {
	t.Helper()

	var out any

	err := p.run(context.Background(), proto, func(v lua.LValue) error {
		converted, err := fromLua(v, defaultBudget())
		if err != nil {
			return err
		}

		out = converted

		return nil
	})

	return out, err
}

// runChunk compiles and runs one chunk on a pooled state.
func runChunk(t *testing.T, p *statePool, src string) (any, error) {
	t.Helper()

	return runProto(t, p, mustCompile(t, src))
}

// callScript runs a chunk on one specific state, which is how a test says "the
// same state as last time".
func callScript(t *testing.T, s *pooledState, src string) (lua.LValue, error) {
	t.Helper()

	return s.call(context.Background(), s.instantiate(mustCompile(t, src)))
}

// TestNewPool_RejectsAnEmptyPool covers the constructor's only argument check.
func TestNewPool_RejectsAnEmptyPool(t *testing.T) {
	t.Parallel()

	if _, err := newPool(0, nil); err == nil {
		t.Fatal("a pool with room for no idle states was accepted")
	}
}

// TestNewPool_FailsWhenPrepareFails checks that a broken host installation is
// reported at boot rather than on the first player's message.
func TestNewPool_FailsWhenPrepareFails(t *testing.T) {
	t.Parallel()

	want := errors.New("no")

	_, err := newPool(2, func(*lua.LState) error { return want })
	if !errors.Is(err, want) {
		t.Fatalf("newPool error = %v, want it to wrap %v", err, want)
	}
}

// TestPool_ReusesAnIdleState is the reason the pool exists at all.
func TestPool_ReusesAnIdleState(t *testing.T) {
	t.Parallel()

	p := newTestPool(t, 4, nil)

	first, err := p.acquire()
	if err != nil {
		t.Fatalf("acquiring: %v", err)
	}

	p.release(first)

	second, err := p.acquire()
	if err != nil {
		t.Fatalf("re-acquiring: %v", err)
	}

	if first.L != second.L {
		t.Fatal("the pool built a new state instead of reusing the idle one")
	}

	p.release(second)
}

// TestPool_KeepsAtMostMaxIdleStates checks the retention bound: the pool may
// hand out as many states as it is asked for, but it keeps only maxIdle.
func TestPool_KeepsAtMostMaxIdleStates(t *testing.T) {
	t.Parallel()

	const maxIdle = 2

	p := newTestPool(t, maxIdle, nil)

	var states []*pooledState

	for range 5 {
		s, err := p.acquire()
		if err != nil {
			t.Fatalf("acquiring: %v", err)
		}

		states = append(states, s)
	}

	for _, s := range states {
		p.release(s)
	}

	if got := idleCount(p); got != maxIdle {
		t.Fatalf("the pool kept %d idle states, want %d", got, maxIdle)
	}

	closed := 0

	for _, s := range states {
		if s.L.IsClosed() {
			closed++
		}
	}

	if want := len(states) - maxIdle; closed != want {
		t.Fatalf("%d released states were closed, want %d", closed, want)
	}
}

// TestPool_CloseClosesIdleStates covers the reason this is not a sync.Pool: an
// LState that is dropped instead of closed never releases what it holds.
func TestPool_CloseClosesIdleStates(t *testing.T) {
	t.Parallel()

	p, err := newPool(2, nil)
	if err != nil {
		t.Fatalf("building a state pool: %v", err)
	}

	s, err := p.acquire()
	if err != nil {
		t.Fatalf("acquiring: %v", err)
	}

	p.release(s)
	p.close()

	if !s.L.IsClosed() {
		t.Fatal("close left an idle state open")
	}

	if _, err := p.acquire(); !errors.Is(err, errPoolClosed) {
		t.Fatalf("acquire after close returned %v, want errPoolClosed", err)
	}
}

// TestPool_CloseClosesAStateReleasedLater covers the invocation that was still
// running when the server shut down.
func TestPool_CloseClosesAStateReleasedLater(t *testing.T) {
	t.Parallel()

	p, err := newPool(2, nil)
	if err != nil {
		t.Fatalf("building a state pool: %v", err)
	}

	s, err := p.acquire()
	if err != nil {
		t.Fatalf("acquiring: %v", err)
	}

	p.close()
	p.release(s)

	if !s.L.IsClosed() {
		t.Fatal("a state released after close was pooled instead of closed")
	}
}

// TestPool_PrepareRunsBeforeTheFreeze proves the ordering build() promises:
// what the host installs is readable by a script, and is read-only afterwards.
func TestPool_PrepareRunsBeforeTheFreeze(t *testing.T) {
	t.Parallel()

	p := newTestPool(t, 2, func(L *lua.LState) error {
		L.SetGlobal("hostValue", lua.LString("installed"))

		return nil
	})

	got, err := runChunk(t, p, `return hostValue`)
	if err != nil {
		t.Fatalf("reading the installed global: %v", err)
	}

	if got != "installed" {
		t.Fatalf("hostValue is %v, want %q", got, "installed")
	}

	if _, err := runChunk(t, p, `hostValue = "tampered" ; _G.hostValue = "tampered"`); err == nil {
		t.Fatal("a script overwrote a host-installed global")
	}
}

// TestPool_AssignedGlobalsDoNotSurviveTheInvocation is the central criterion of
// this story, and it is asserted against *the same state object* rather than
// against whatever the pool happens to hand out next.
func TestPool_AssignedGlobalsDoNotSurviveTheInvocation(t *testing.T) {
	t.Parallel()

	p := newTestPool(t, 2, nil)

	s, err := p.acquire()
	if err != nil {
		t.Fatalf("acquiring: %v", err)
	}

	defer p.release(s)

	// The write has to be visible to the invocation that made it, or the
	// isolation would be indistinguishable from a broken interpreter.
	ret, err := callScript(t, s, `leaked = "yes" ; return leaked`)
	if err != nil {
		t.Fatalf("assigning a global: %v", err)
	}

	if got := lua.LVAsString(ret); got != "yes" {
		t.Fatalf("the invocation read back %q from its own global, want %q", got, "yes")
	}

	ret, err = callScript(t, s, `return leaked`)
	if err != nil {
		t.Fatalf("reading the global back on the same state: %v", err)
	}

	if ret != lua.LNil {
		t.Fatalf("the next invocation on the same state saw %s, want nil", ret.String())
	}
}

// TestPool_FreezeRefusesEveryWriteToSharedState walks the routes by which a
// script could reach something the next invocation on its state would see.
//
// Each case is asserted twice: the write is refused, and the shared value is
// still what it was. The second assertion is the one that matters — a refusal
// that happened after the write would leak just the same.
func TestPool_FreezeRefusesEveryWriteToSharedState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		write string
		probe string
		want  any
	}{
		{"new global through _G", `_G.planted = "yes"`, `return _G.planted`, nil},
		{"existing global through _G", `_G.pairs = nil`, `return type(pairs)`, "function"},
		{"overwrite a library function", `string.format = nil`, `return type(string.format)`, "function"},
		{"add to a library table", `string.planted = "yes"`, `return string.planted`, nil},
		{"overwrite another library", `math.floor = nil`, `return type(math.floor)`, "function"},
		{"replace a loaded module", `package.loaded.string = {}`, `return package.loaded.string == string`, true},
		{"add a loaded module", `package.loaded.planted = {}`, `return package.loaded.planted`, nil},
		{"add a preloaded module", `package.preload.planted = function() end`, `return package.preload.planted`, nil},
		{"replace a module searcher", `package.loaders[1] = "planted"`, `return type(package.loaders[1])`, "function"},
		{"reseat package.loaded", `package.loaded = {}`, `return package.loaded.string == string`, true},
		// table.insert writes through the array part with a raw set, which no
		// metatable can intercept.
		{"insert into a library table", `table.insert(string, "planted")`, `return string[1]`, nil},
		{"insert into the globals table", `table.insert(_G, "planted")`, `return _G[1]`, nil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			p := newTestPool(t, 1, nil)

			s, err := p.acquire()
			if err != nil {
				t.Fatalf("acquiring: %v", err)
			}

			defer p.release(s)

			if _, err := callScript(t, s, test.write); err == nil {
				t.Fatalf("%s was allowed", test.write)
			}

			ret, err := callScript(t, s, test.probe)
			if err != nil {
				t.Fatalf("probing with %s: %v", test.probe, err)
			}

			got, err := fromLua(ret, defaultBudget())
			if err != nil {
				t.Fatalf("converting the probe result: %v", err)
			}

			if got != test.want {
				t.Fatalf("%s yielded %v after the refused write, want %v", test.probe, got, test.want)
			}
		})
	}
}

// TestPool_FreezeLeavesTheLibrariesUsable is the negative control for the
// freeze. A state that refused everything would pass the test above and be
// useless.
func TestPool_FreezeLeavesTheLibrariesUsable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		script string
		want   any
	}{
		{"library call", `return string.format("%s-%d", "x", 2)`, "x-2"},
		{"method syntax on a string", `return ("ab"):rep(2)`, "abab"},
		{"the capped library is still capped", `return pcall(string.rep, "x", 1e9)`, false},
		{"another library", `return math.floor(2.7)`, float64(2)},
		{"a base function", `return select("#", 1, 2, 3)`, float64(3)},
		{"reading a global through _G", `return type(_G.pcall)`, "function"},
		{"require returns the frozen module", `return require("string") == string`, true},
		{"table.insert into a script's own table", `local t = {} table.insert(t, "x") return t[1]`, "x"},
		{"a local variable", `local n = 0 for i = 1, 3 do n = n + i end return n`, float64(6)},
		{"writing a global the script owns", `own = 5 return own`, float64(5)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			p := newTestPool(t, 1, nil)

			got, err := runChunk(t, p, test.script)
			if err != nil {
				t.Fatalf("running %s: %v", test.script, err)
			}

			if got != test.want {
				t.Fatalf("%s returned %v, want %v", test.script, got, test.want)
			}
		})
	}
}

// TestPool_FreezesEveryStateItBuilds guards the ordering in build(): the state
// newPool makes eagerly and the ones acquire makes on demand go through exactly
// the same preparation.
func TestPool_FreezesEveryStateItBuilds(t *testing.T) {
	t.Parallel()

	p := newTestPool(t, 4, nil)

	first, err := p.acquire()
	if err != nil {
		t.Fatalf("acquiring the eagerly built state: %v", err)
	}

	defer p.release(first)

	// Nothing is idle now, so this one is built on demand.
	second, err := p.acquire()
	if err != nil {
		t.Fatalf("acquiring a state built on demand: %v", err)
	}

	defer p.release(second)

	for name, s := range map[string]*pooledState{"eager": first, "on demand": second} {
		t.Run(name, func(t *testing.T) {
			if _, err := callScript(t, s, `string.format = nil`); err == nil {
				t.Fatal("the state let a script overwrite a library function")
			}
		})
	}
}

// TestPool_DiscardsAStateKilledByItsDeadline is the correction this story is
// built on: a state interrupted mid-instruction never unwound its stack, so it
// must not serve another player.
func TestPool_DiscardsAStateKilledByItsDeadline(t *testing.T) {
	t.Parallel()

	p := newTestPool(t, 4, nil)

	s, err := p.acquire()
	if err != nil {
		t.Fatalf("acquiring: %v", err)
	}

	killed := s.L

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	if _, err := s.call(ctx, s.instantiate(mustCompile(t, `while true do end`))); err == nil {
		t.Fatal("the infinite loop returned without an error")
	}

	if ctx.Err() == nil {
		t.Fatal("the script failed before its deadline expired")
	}

	if !s.spoiled {
		t.Fatal("a state killed by its deadline was not marked spoiled")
	}

	p.release(s)

	if got := idleCount(p); got != 0 {
		t.Fatalf("the pool holds %d idle states, want the killed one discarded", got)
	}

	if !killed.IsClosed() {
		t.Fatal("the killed state was not closed")
	}

	next, err := p.acquire()
	if err != nil {
		t.Fatalf("acquiring after the kill: %v", err)
	}

	defer p.release(next)

	if next.L == killed {
		t.Fatal("the pool handed out the killed state again")
	}
}

// TestPool_DiscardsAStateAfterAPanic covers the ApiErrorPanic case: a Go panic
// crossed the interpreter, and nothing is known about what it left behind.
func TestPool_DiscardsAStateAfterAPanic(t *testing.T) {
	t.Parallel()

	p := newTestPool(t, 4, func(L *lua.LState) error {
		L.SetGlobal("boom", L.NewFunction(func(*lua.LState) int {
			panic(errors.New("a host function came apart"))
		}))

		return nil
	})

	s, err := p.acquire()
	if err != nil {
		t.Fatalf("acquiring: %v", err)
	}

	if _, err := callScript(t, s, `boom()`); err == nil {
		t.Fatal("the panicking host function returned without an error")
	}

	if !s.spoiled {
		t.Fatal("a state that survived a Go panic was not marked spoiled")
	}

	p.release(s)

	if !s.L.IsClosed() {
		t.Fatal("the state was pooled after a panic")
	}
}

// TestPool_DiscardsAnExhaustedState covers the third contaminating failure. A
// state that has grown its stack to the configured maximum would carry that
// memory for the rest of its life.
func TestPool_DiscardsAnExhaustedState(t *testing.T) {
	t.Parallel()

	p := newTestPool(t, 4, nil)

	s, err := p.acquire()
	if err != nil {
		t.Fatalf("acquiring: %v", err)
	}

	// Not a tail call: "return 1 + f()" has to keep every frame.
	_, err = callScript(t, s, `local function f() return 1 + f() end return f()`)
	if err == nil {
		t.Fatal("unbounded recursion returned without an error")
	}

	if !strings.Contains(err.Error(), "overflow") {
		t.Fatalf("recursion failed with %v, want an overflow", err)
	}

	if !s.spoiled {
		t.Fatal("an exhausted state was not marked spoiled")
	}

	p.release(s)

	if !s.L.IsClosed() {
		t.Fatal("the state was pooled after exhausting its stack")
	}
}

// TestPool_KeepsAStateAfterAnOrdinaryScriptError is the negative control for
// spoils. PCall unwinds an ordinary error completely, and rebuilding a state
// for every typo in a game script would be a rebuild per message.
func TestPool_KeepsAStateAfterAnOrdinaryScriptError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		script string
	}{
		{"error", `error("no")`},
		{"failed assert", `assert(false, "no")`},
		{"indexing a nil", `local t = nil return t.field`},
		{"calling a nil", `undefinedFunction()`},
		{"a rejected string cap", `return string.rep("x", 1e9)`},
		{"an error object that is not a string", `error({code = 1})`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			p := newTestPool(t, 2, nil)

			s, err := p.acquire()
			if err != nil {
				t.Fatalf("acquiring: %v", err)
			}

			if _, err := callScript(t, s, test.script); err == nil {
				t.Fatalf("%s returned without an error", test.script)
			}

			if s.spoiled {
				t.Fatalf("%s spoiled the state", test.script)
			}

			p.release(s)

			if idleCount(p) != 1 {
				t.Fatal("the state was discarded after an ordinary script error")
			}

			// And it still works.
			got, err := runChunk(t, p, `return 1 + 1`)
			if err != nil {
				t.Fatalf("reusing the state: %v", err)
			}

			if got != float64(2) {
				t.Fatalf("the reused state returned %v, want 2", got)
			}
		})
	}
}

// TestPool_FiftyConcurrentInvocations is the race-detector criterion, and it
// asserts isolation at the same time: every invocation increments a global that
// starts out unset, so a result other than 1 means one invocation saw another's
// environment.
func TestPool_FiftyConcurrentInvocations(t *testing.T) {
	t.Parallel()

	const (
		goroutines = 50
		iterations = 10
	)

	p := newTestPool(t, 8, nil)

	// One proto, shared by every goroutine: bytecode is the only thing that may
	// cross a state boundary.
	proto := mustCompile(t, `counter = (counter or 0) + 1 return counter`)

	failures := make(chan error, goroutines*iterations)

	var wg sync.WaitGroup

	for range goroutines {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for range iterations {
				got, err := runProto(t, p, proto)
				if err != nil {
					failures <- fmt.Errorf("invoking: %w", err)

					continue
				}

				if got != float64(1) {
					failures <- fmt.Errorf("an invocation counted %v, want 1", got)
				}
			}
		}()
	}

	wg.Wait()
	close(failures)

	for err := range failures {
		t.Error(err)
	}

	if got := idleCount(p); got > 8 {
		t.Fatalf("the pool kept %d idle states, want at most 8", got)
	}
}

// TestSpoils covers the decision directly, including the failures that must
// *not* discard a state.
func TestSpoils(t *testing.T) {
	t.Parallel()

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	runError := errors.New("a plain go error")

	tests := []struct {
		name string
		ctx  context.Context
		err  error
		want bool
	}{
		{"a cancelled context", cancelled, runError, true},
		{"a go panic", context.Background(), &lua.ApiError{Type: lua.ApiErrorPanic, Object: lua.LString("boom")}, true},
		{"a stack overflow", context.Background(), &lua.ApiError{Type: lua.ApiErrorRun, Object: lua.LString("game.lua:2: stack overflow")}, true},
		{"a registry overflow", context.Background(), &lua.ApiError{Type: lua.ApiErrorRun, Object: lua.LString("registry overflow")}, true},
		{"an ordinary run error", context.Background(), &lua.ApiError{Type: lua.ApiErrorRun, Object: lua.LString("game.lua:2: no")}, false},
		{"an error object that is a table", context.Background(), &lua.ApiError{Type: lua.ApiErrorRun, Object: &lua.LTable{}}, false},
		{"an error from outside the interpreter", context.Background(), runError, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := spoils(test.ctx, test.err); got != test.want {
				t.Fatalf("spoils() = %t, want %t", got, test.want)
			}
		})
	}
}
