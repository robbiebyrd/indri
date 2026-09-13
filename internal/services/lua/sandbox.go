package lua

import (
	"errors"
	"fmt"
	"strconv"

	lua "github.com/yuin/gopher-lua"
)

// defaultMaxStringBytes bounds the output of the two string functions a script
// can use to inflate a small input into an arbitrarily large one. It is a cap on
// one call, not on total allocation — see the limitation noted on newState.
const defaultMaxStringBytes = 1 << 20 // 1 MiB

// sandboxLibs is the whole standard library a script may see, in open order.
//
// package comes first because OpenBase installs require, and PreloadModule
// raises if the package table does not exist yet. io, os, debug, coroutine and
// channel are absent on purpose: they reach the filesystem, the process
// environment, the Go call stack and the scheduler respectively, none of which
// a game script has any business touching.
var sandboxLibs = []struct {
	name string
	open lua.LGFunction
}{
	{lua.LoadLibName, lua.OpenPackage},
	{lua.BaseLibName, lua.OpenBase},
	{lua.TabLibName, lua.OpenTable},
	{lua.StringLibName, lua.OpenString},
	{lua.MathLibName, lua.OpenMath},
}

// unsafeGlobals is every base-library global removed after OpenBase.
//
// gopher-lua has no sandbox mode (issue #27): the dangerous functions are
// installed by OpenBase and have to be taken back out by hand. They fall into
// four groups.
//
// Filesystem and code loading — dofile, loadfile, load, loadstring — read a
// chunk from disk or from a string and run it. load and loadstring also accept
// a *binary* chunk, and gopher-lua does not verify hand-crafted bytecode, so
// removing them is what makes "no binary chunks" true by construction.
//
// Host reach — collectgarbage forces a process-wide runtime.GC; print and
// _printregs write to the server's stdout, where a script's output would be
// indistinguishable from the server's own log.
//
// Metatable and environment control — setmetatable, getmetatable, rawset,
// rawget, newproxy, setfenv, getfenv. The raw* pair bypasses any __index or
// __newindex guard the host installs, newproxy mints userdata with a metatable
// the script controls, and the fenv pair reassigns a function's environment,
// which would let a script step out of the per-invocation globals table.
//
// Module declaration — module rewrites the calling chunk's environment and
// publishes globals as a side effect. require stays: it is how a preloaded
// in-memory module is reached.
var unsafeGlobals = []string{
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
	"setfenv",
	"getfenv",
	"module",
	"_printregs",
}

// newState builds an LState with the standard library cut down to what a game
// script needs, and the two uninterruptible allocation bombs capped.
//
// The caller owns the returned state and must Close it.
//
// Two limitations are accepted rather than solved, and neither should be
// described to an operator as a guarantee:
//
// Heap. gopher-lua has no allocator hook (issue #230), so nothing here bounds
// memory. RegistryMaxSize bounds the value stack only, and overflowing it
// raises an ordinary run error rather than unwinding an allocation. A script
// that grows a table forever can still exhaust the process. The trust tier is
// operator-authored, which is why that is acceptable.
//
// Time. L.SetContext interrupts the VM between main-loop iterations, so a
// deadline stops a Lua loop but cannot stop one long call into a Go library
// function (issue #521). string.rep and string.format are the two such calls a
// script can make arbitrarily expensive from a tiny source, so their size caps
// are the timeout on those paths, not a convenience.
func newState(maxStringBytes int) (*lua.LState, error) {
	L := lua.NewState(lua.Options{
		SkipOpenLibs:        true,
		RegistrySize:        1024 * 20,
		RegistryMaxSize:     1024 * 80,
		RegistryGrowStep:    32,
		CallStackSize:       256,
		MinimizeStackMemory: true,
		// A Go stack trace in a script error would leak host internals to the
		// client that triggered it.
		IncludeGoStackTrace: false,
	})

	for _, lib := range sandboxLibs {
		L.Push(L.NewFunction(lib.open))
		L.Push(lua.LString(lib.name))

		if err := L.PCall(1, 0, nil); err != nil {
			L.Close()

			return nil, fmt.Errorf("opening the %q library: %w", lib.name, err)
		}
	}

	for _, name := range unsafeGlobals {
		L.SetGlobal(name, lua.LNil)
	}

	if err := sealPackage(L); err != nil {
		L.Close()

		return nil, err
	}

	if err := capStringLib(L, maxStringBytes); err != nil {
		L.Close()

		return nil, err
	}

	return L, nil
}

// defaultState builds a state with the default caps.
func defaultState() (*lua.LState, error) {
	return newState(defaultMaxStringBytes)
}

// sealPackage cuts require's route to the filesystem.
//
// OpenPackage seeds package.path from the LUA_PATH environment variable, so on
// a host that sets it a require would search real directories. Emptying path
// and cpath leaves the preload searcher — which serves modules the host put in
// memory — as the only one that can ever find anything. loadlib is nil'd for
// the same reason, even though gopher-lua's implementation only raises: a
// script must not be able to name a shared object at all.
func sealPackage(L *lua.LState) error {
	pkg, ok := L.GetGlobal("package").(*lua.LTable)
	if !ok {
		return fmt.Errorf("package table is missing after opening %q", lua.LoadLibName)
	}

	pkg.RawSetString("path", lua.LString(""))
	pkg.RawSetString("cpath", lua.LString(""))
	pkg.RawSetString("loadlib", lua.LNil)

	return nil
}

// capStringLib replaces string.rep and string.format with wrappers that refuse
// a call whose output would exceed maxBytes, then delegate to the real
// function.
//
// These two are special because their cost is set by an *argument*, not by the
// size of the source: string.rep("x", 1e9) and string.format("%1000000000d", 1)
// are both one bytecode instruction that allocates a gigabyte inside a single Go
// call the context deadline cannot interrupt.
//
// Patching the string table is enough to cover method syntax as well. OpenString
// stores the same table as the string metatable's __index, so ("x"):rep(n)
// resolves to whatever string.rep holds now.
func capStringLib(L *lua.LState, maxBytes int) error {
	strlib, ok := L.GetGlobal("string").(*lua.LTable)
	if !ok {
		return fmt.Errorf("string table is missing after opening %q", lua.StringLibName)
	}

	rep := strlib.RawGetString("rep")
	format := strlib.RawGetString("format")

	strlib.RawSetString("rep", L.NewFunction(func(L *lua.LState) int {
		s, n := L.CheckString(1), L.CheckInt(2)

		// Written as a division so the product is never formed: n*len(s) would
		// overflow for exactly the arguments this exists to reject.
		if n > 0 && len(s) > 0 && n > maxBytes/len(s) {
			L.RaiseError("string.rep would produce %d x %d bytes, over the %d byte cap", n, len(s), maxBytes)
		}

		return delegate(L, rep)
	}))

	strlib.RawSetString("format", L.NewFunction(func(L *lua.LState) int {
		if err := checkFormat(L.CheckString(1), maxBytes); err != nil {
			L.RaiseError("string.format: %s", err.Error())
		}

		return delegate(L, format)
	}))

	return nil
}

// delegate calls fn with the arguments the wrapper was given and yields its
// single result.
//
// L.Call is unprotected on purpose: an error raised inside the real function
// must reach the script's own pcall, exactly as it would have without the
// wrapper.
func delegate(L *lua.LState, fn lua.LValue) int {
	top := L.GetTop()

	L.Push(fn)

	for i := 1; i <= top; i++ {
		L.Push(L.Get(i))
	}

	L.Call(top, 1)

	return 1
}

// checkFormat rejects a format string whose declared field widths alone would
// exceed maxBytes.
//
// The width and precision fields are the part of the output a script controls
// without supplying a correspondingly large argument, which is what makes them
// the bomb: "%1000000000d" is twelve source bytes and a gigabyte of padding.
// The bound is deliberately loose — it counts only literal bytes and the larger
// of each field's width and precision, and ignores how long the formatted
// argument itself turns out to be.
//
// That looseness is the honest limit of this check rather than an oversight. A
// large %s argument still passes, because a string that big had to be built
// first, and Lua's own concatenation operator is uncapped anyway. Capping the
// result after the fact would not prevent the allocation it is supposed to
// prevent.
func checkFormat(format string, maxBytes int) error {
	total := 0

	for i := 0; i < len(format); {
		if format[i] != '%' {
			total++
			i++

			continue
		}

		i++

		// "%%" is a literal percent sign and carries no field.
		if i < len(format) && format[i] == '%' {
			total++
			i++

			continue
		}

		for i < len(format) && isFormatFlag(format[i]) {
			i++
		}

		width, next, err := scanFieldSize(format, i, maxBytes)
		if err != nil {
			return err
		}

		i = next

		precision := 0

		if i < len(format) && format[i] == '.' {
			i++

			if precision, i, err = scanFieldSize(format, i, maxBytes); err != nil {
				return err
			}
		}

		// Each field is already known to be within the cap, and there can be no
		// more directives than there are bytes of format string, so the sum
		// cannot overflow before it is compared.
		total += max(width, precision)

		// The verb itself, whatever it is. An unterminated directive is left for
		// the real function to report.
		if i < len(format) {
			i++
		}
	}

	if total > maxBytes {
		return fmt.Errorf("output would be at least %d bytes, over the %d byte cap", total, maxBytes)
	}

	return nil
}

func isFormatFlag(c byte) bool {
	switch c {
	case '-', '+', ' ', '#', '0':
		return true
	default:
		return false
	}
}

// scanFieldSize reads one width or precision field and reports the index just
// past it.
//
// A '*' takes the field size from an argument, which no static check can bound.
// Lua 5.1's string.format does not support it, so refusing it costs nothing and
// closes the only way around this check.
func scanFieldSize(format string, i, maxBytes int) (int, int, error) {
	if i < len(format) && format[i] == '*' {
		return 0, i, errors.New("'*' field width is not supported")
	}

	start := i
	for i < len(format) && format[i] >= '0' && format[i] <= '9' {
		i++
	}

	if i == start {
		return 0, i, nil
	}

	// A field wide enough to overflow an int is, a fortiori, over the cap.
	size, err := strconv.Atoi(format[start:i])
	if err != nil || size > maxBytes {
		return 0, i, fmt.Errorf("field width %s exceeds the %d byte cap", format[start:i], maxBytes)
	}

	return size, i, nil
}
