package lua

import (
	"context"
	"strings"
	"testing"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// newTestState builds a sandboxed state with the default caps and closes it
// when the test ends.
func newTestState(t *testing.T) *lua.LState {
	t.Helper()

	L, err := defaultState()
	if err != nil {
		t.Fatalf("building the sandboxed state: %v", err)
	}

	t.Cleanup(L.Close)

	return L
}

// TestNewState_StripsUnsafeGlobals pins the removal list down by name rather
// than by iterating unsafeGlobals, so shortening that slice fails here instead
// of quietly reopening a hole.
func TestNewState_StripsUnsafeGlobals(t *testing.T) {
	t.Parallel()

	names := []string{
		"dofile",
		"loadfile",
		"load",
		"loadstring",
		"collectgarbage",
		"print",
		"setmetatable",
		"getmetatable",
		"rawset",
		"rawget",
		"newproxy",
		// Not required by the story, removed for the same reasons: both reassign
		// a function's environment, which would defeat per-invocation globals.
		"setfenv",
		"getfenv",
		// Rewrites the calling chunk's environment and publishes globals.
		"module",
		// Writes VM registers to the server's stdout.
		"_printregs",
	}

	L := newTestState(t)

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			if got := L.GetGlobal(name); got != lua.LNil {
				t.Fatalf("global %q is %s, want nil", name, got.Type())
			}
		})
	}
}

// TestNewState_OmitsUnsafeLibraries checks the libraries that are never opened
// at all. A stripped global can be restored by name; a library that was never
// opened has no table to restore.
func TestNewState_OmitsUnsafeLibraries(t *testing.T) {
	t.Parallel()

	L := newTestState(t)

	for _, name := range []string{"os", "io", "debug", "coroutine", "channel"} {
		t.Run(name, func(t *testing.T) {
			if got := L.GetGlobal(name); got != lua.LNil {
				t.Fatalf("library table %q is %s, want nil", name, got.Type())
			}
		})
	}
}

// TestNewState_OpensAllowedLibraries is the negative control for the two tests
// above: the strip must leave a usable Lua, not a husk.
func TestNewState_OpensAllowedLibraries(t *testing.T) {
	t.Parallel()

	L := newTestState(t)

	for _, name := range []string{
		"package", "string", "table", "math",
		"pairs", "ipairs", "pcall", "xpcall", "error", "assert",
		"tostring", "tonumber", "type", "select", "next", "unpack", "require",
	} {
		t.Run(name, func(t *testing.T) {
			if got := L.GetGlobal(name); got == lua.LNil {
				t.Fatalf("%q is nil, want it available", name)
			}
		})
	}

	if err := L.DoString(`
		local t = {}
		for i = 1, 3 do t[#t + 1] = string.format("%d:%s", i, math.floor(1.5)) end
		assert(table.concat(t, ",") == "1:1,2:1,3:1", table.concat(t, ","))
	`); err != nil {
		t.Fatalf("running a script over the remaining libraries: %v", err)
	}
}

// TestNewState_SealsPackage proves the filesystem searchers cannot fire even as
// a fallback: with both search paths empty there is nowhere on disk for require
// to look, and loadlib cannot name a shared object.
func TestNewState_SealsPackage(t *testing.T) {
	t.Parallel()

	L := newTestState(t)

	pkg, ok := L.GetGlobal("package").(*lua.LTable)
	if !ok {
		t.Fatalf("package is %s, want a table", L.GetGlobal("package").Type())
	}

	for _, field := range []string{"path", "cpath"} {
		t.Run(field, func(t *testing.T) {
			got := pkg.RawGetString(field)

			s, ok := got.(lua.LString)
			if !ok {
				t.Fatalf("package.%s is %s, want a string", field, got.Type())
			}

			if s != "" {
				t.Fatalf("package.%s is %q, want empty", field, string(s))
			}
		})
	}

	t.Run("loadlib", func(t *testing.T) {
		if got := pkg.RawGetString("loadlib"); got != lua.LNil {
			t.Fatalf("package.loadlib is %s, want nil", got.Type())
		}
	})

	t.Run("require finds nothing on disk", func(t *testing.T) {
		if err := L.DoString(`require("os")`); err == nil {
			t.Fatal("require succeeded, want a not-found error")
		}
	})
}

// TestNewState_ContextDeadlineStopsAnInfiniteLoop covers the interruptible
// path: a Lua loop yields to the VM main loop on every iteration, which is
// where SetContext checks the deadline.
//
// The failure is asserted through ctx.Err(), never by matching the error text.
// A deadline surfaces as an ordinary run error indistinguishable from a script
// calling error() itself.
func TestNewState_ContextDeadlineStopsAnInfiniteLoop(t *testing.T) {
	t.Parallel()

	L := newTestState(t)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	L.SetContext(ctx)

	start := time.Now()

	err := L.DoString(`while true do end`)
	if err == nil {
		t.Fatal("the infinite loop returned without an error, want it killed by the deadline")
	}

	if ctx.Err() == nil {
		t.Fatalf("the script failed before the deadline expired: %v", err)
	}

	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("the deadline took %v to take effect", elapsed)
	}
}

// TestNewState_StringCapsRejectAllocationBombs is the load-bearing test of this
// file. A single call into a Go library function is not interruptible by the
// context deadline, so if these calls were allowed to start, nothing would stop
// them.
//
// Every case is asserted to fail *quickly*: a test that merely proved an error
// would still pass if the wrapper allocated the gigabyte first and then
// complained about it.
func TestNewState_StringCapsRejectAllocationBombs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		script string
	}{
		{"rep by count", `return string.rep("x", 1e9)`},
		{"rep by repeated unit", `return string.rep(string.rep("x", 1000), 1e6)`},
		{"rep through method syntax", `return ("x"):rep(1e9)`},
		{"format width", `return string.format("%2000000000d", 1)`},
		{"format precision", `return string.format("%.2000000000f", 1)`},
		{"format width beyond an int", `return string.format("%999999999999999999999d", 1)`},
		{"format widths summing over the cap", `return string.format("%600000s%600000s", "a", "b")`},
		// '*' takes its width from an argument, which no static check can bound.
		{"format star width", `return string.format("%*d", 2000000000, 1)`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			L := newTestState(t)

			start := time.Now()

			if err := L.DoString(test.script); err == nil {
				t.Fatal("the script succeeded, want the size cap to reject it")
			}

			if elapsed := time.Since(start); elapsed > time.Second {
				t.Fatalf("the cap took %v to reject the call, so it allocated first", elapsed)
			}
		})
	}
}

// TestNewState_StringCapsAllowOrdinaryUse is the negative control for the caps.
// A cap that broke everyday formatting would be worse than no cap at all,
// because a game script would work around it.
func TestNewState_StringCapsAllowOrdinaryUse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		script string
		want   string
	}{
		{"small rep", `return string.rep("ab", 3)`, "ababab"},
		{"rep through method syntax", `return ("-"):rep(5)`, "-----"},
		{"rep of zero", `return string.rep("x", 0)`, ""},
		{"rep of a negative count", `return string.rep("x", -4)`, ""},
		{"rep of an empty string", `return string.rep("", 1e9)`, ""},
		{"rep right up to the cap", `return #string.rep("x", 1048576) .. ""`, "1048576"},
		{"plain format", `return string.format("%s scored %d", "ada", 7)`, "ada scored 7"},
		{"format with a width", `return string.format("[%6.2f]", 3.14159)`, "[  3.14]"},
		{"format with a literal percent", `return string.format("%d%%", 50)`, "50%"},
		{"format with flags", `return string.format("%-5s|%05d|", "x", 42)`, "x    |00042|"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			L := newTestState(t)

			if err := L.DoString(test.script); err != nil {
				t.Fatalf("running %s: %v", test.script, err)
			}

			got, ok := L.Get(-1).(lua.LString)
			if !ok {
				t.Fatalf("result is %s, want a string", L.Get(-1).Type())
			}

			if string(got) != test.want {
				t.Fatalf("got %q, want %q", string(got), test.want)
			}
		})
	}
}

// TestNewState_StringCapErrorsAreCatchable checks that a rejected call raises
// an ordinary Lua error rather than a Go panic, so a script's own pcall sees it
// the way it would see any other library error.
func TestNewState_StringCapErrorsAreCatchable(t *testing.T) {
	t.Parallel()

	L := newTestState(t)

	if err := L.DoString(`
		local ok, err = pcall(function() return string.rep("x", 1e9) end)
		assert(ok == false, "the call was not rejected")
		assert(type(err) == "string", "the error is not a string")
	`); err != nil {
		t.Fatalf("catching the cap error from Lua: %v", err)
	}
}

// TestNewState_StringCapRespectsItsArgument checks the cap is the configured
// number and not a constant baked into the wrapper.
func TestNewState_StringCapRespectsItsArgument(t *testing.T) {
	t.Parallel()

	L, err := newState(16)
	if err != nil {
		t.Fatalf("building a state with a 16 byte cap: %v", err)
	}

	t.Cleanup(L.Close)

	if err := L.DoString(`return string.rep("x", 16)`); err != nil {
		t.Fatalf("a 16 byte result was rejected by a 16 byte cap: %v", err)
	}

	if err := L.DoString(`return string.rep("x", 17)`); err == nil {
		t.Fatal("a 17 byte result was allowed by a 16 byte cap")
	}
}

// TestCheckFormat covers the bound directly, including the shapes a script
// could use to hide a large field from a naive scan.
func TestCheckFormat(t *testing.T) {
	t.Parallel()

	const cap = 1000

	tests := []struct {
		name    string
		format  string
		wantErr bool
	}{
		{"no directives", strings.Repeat("a", 999), false},
		{"literal bytes over the cap", strings.Repeat("a", 1001), true},
		{"bare verb", "%d", false},
		{"escaped percent", "%%2000000000d", false},
		{"width inside the cap", "%999d", false},
		{"width over the cap", "%1001d", true},
		{"precision over the cap", "%.1001f", true},
		{"flags before the width", "%-0+ #1001d", true},
		{"two widths that only sum over the cap", "%600d%600d", true},
		{"star width", "%*d", true},
		{"star precision", "%.*f", true},
		{"unterminated directive", "%", false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := checkFormat(test.format, cap)
			if (err != nil) != test.wantErr {
				t.Fatalf("checkFormat(%q) error = %v, wantErr %t", test.format, err, test.wantErr)
			}
		})
	}
}
