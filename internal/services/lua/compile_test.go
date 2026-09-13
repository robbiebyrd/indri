package lua

import (
	"context"
	"strings"
	"testing"

	lua "github.com/yuin/gopher-lua"
)

// mustCompile compiles a chunk that the test asserts is valid.
func mustCompile(t *testing.T, src string) *lua.FunctionProto {
	t.Helper()

	proto, err := compile("test.lua", src)
	if err != nil {
		t.Fatalf("compiling %q: %v", src, err)
	}

	return proto
}

// TestCompile_ReportsASyntaxErrorWithItsPosition checks the one thing a script
// author needs from a failed compile: which file, and which line.
func TestCompile_ReportsASyntaxErrorWithItsPosition(t *testing.T) {
	t.Parallel()

	_, err := compile("game.lua", "local ok = true\nlocal 1 = 2\nreturn ok\n")
	if err == nil {
		t.Fatal("compiling a chunk with a malformed local succeeded")
	}

	for _, want := range []string{"game.lua", ":2"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not mention %q", err.Error(), want)
		}
	}
}

// TestProtoCache_CompilesEachChunkOnce is the compile-cache criterion: the same
// source under the same name is compiled once and the identical bytecode is
// handed out afterwards.
func TestProtoCache_CompilesEachChunkOnce(t *testing.T) {
	t.Parallel()

	const src = `return 1 + 1`

	cache := newProtoCache()

	first, err := cache.compile("game.lua", src)
	if err != nil {
		t.Fatalf("first compile: %v", err)
	}

	second, err := cache.compile("game.lua", src)
	if err != nil {
		t.Fatalf("second compile: %v", err)
	}

	if first != second {
		t.Fatal("the cache compiled the same chunk twice")
	}
}

// TestProtoCache_RecompilesChangedSource is the other half of the cache
// contract. Serving bytecode from a previous version of a script would make a
// reload silently do nothing.
func TestProtoCache_RecompilesChangedSource(t *testing.T) {
	t.Parallel()

	cache := newProtoCache()

	first, err := cache.compile("game.lua", `return 1`)
	if err != nil {
		t.Fatalf("compiling the first version: %v", err)
	}

	second, err := cache.compile("game.lua", `return 2`)
	if err != nil {
		t.Fatalf("compiling the second version: %v", err)
	}

	if first == second {
		t.Fatal("the cache served bytecode from the previous source")
	}

	p := newTestPool(t, 2, nil)

	got, err := runProto(t, p, second)
	if err != nil {
		t.Fatalf("running the recompiled chunk: %v", err)
	}

	if got != float64(2) {
		t.Fatalf("the recompiled chunk returned %v, want 2", got)
	}
}

// TestCompile_OneProtoRunsOnEveryState pins down the rule the whole package is
// arranged around: bytecode crosses states, functions do not. The same proto
// runs on several states, and each state builds its own LFunction from it.
func TestCompile_OneProtoRunsOnEveryState(t *testing.T) {
	t.Parallel()

	proto := mustCompile(t, `return "from the shared proto"`)

	p := newTestPool(t, 4, nil)

	first, err := p.acquire()
	if err != nil {
		t.Fatalf("acquiring the first state: %v", err)
	}

	second, err := p.acquire()
	if err != nil {
		t.Fatalf("acquiring the second state: %v", err)
	}

	if first.L == second.L {
		t.Fatal("the pool handed out one state twice")
	}

	fns := map[*lua.LFunction]struct{}{}

	for _, s := range []*pooledState{first, second} {
		fn := s.instantiate(proto)
		fns[fn] = struct{}{}

		ret, err := s.call(context.Background(), fn)
		if err != nil {
			t.Fatalf("running the shared proto: %v", err)
		}

		if got := lua.LVAsString(ret); got != "from the shared proto" {
			t.Fatalf("the shared proto returned %q", got)
		}
	}

	if len(fns) != 2 {
		t.Fatal("both states instantiated the same LFunction, which cannot be shared")
	}

	p.release(first)
	p.release(second)
}
