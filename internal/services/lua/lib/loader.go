// Package lib serves the Lua helper modules that ship with indri.
//
// The modules are written in Lua rather than Go because they need nothing from
// the host: they read a game state that is already a Lua table. Keeping them in
// Lua makes them readable by the people writing game scripts, overridable by a
// game that wants different behaviour, and testable without a host at all.
//
// They are embedded into the binary and served out of memory. A sandboxed state
// has package.path and package.cpath emptied, so package.preload is the only
// searcher require can reach and a module has no path on disk for a script to
// name. Install is what fills that table.
package lib

import (
	_ "embed"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	lua "github.com/yuin/gopher-lua"
	"github.com/yuin/gopher-lua/parse"
)

//go:embed indri/game.lua
var gameSource string

// modules is every shipped module, keyed by the name require asks for.
//
// The key is the module name and not a path. It is what a script writes --
// require("indri.game") -- and what package.preload is keyed by; the file the
// source came from is only where the bytes live.
var modules = map[string]string{
	"indri.game": gameSource,
}

// compiled holds the bytecode for every module, compiled once for the process.
//
// A *lua.FunctionProto is immutable and holds no reference to the state that
// produced it, so one proto serves every state ever built. The alternative --
// parsing the same embedded source on every state the pool creates -- would
// repeat that work for no gain.
var compiled = sync.OnceValues(func() (map[string]*lua.FunctionProto, error) {
	protos := make(map[string]*lua.FunctionProto, len(modules))

	for _, name := range Names() {
		proto, err := compile(name, modules[name])
		if err != nil {
			return nil, err
		}

		protos[name] = proto
	}

	return protos, nil
})

// Names lists the shipped module names, sorted.
func Names() []string {
	return slices.Sorted(maps.Keys(modules))
}

// Install preloads every shipped module on L and requires each one once.
//
// Both halves are needed, and the second is the subtle one. Preloading alone
// leaves the module unloaded, and a state is frozen after it is prepared: the
// first require of a module records the result with a write to package.loaded,
// which a frozen state refuses. Requiring here, while the state is still
// writable, is what makes require("indri.game") resolve from the cache for
// every invocation that state goes on to serve.
//
// It must therefore be called before the state is frozen. On a frozen state the
// preload write itself raises, which outside a protected call is a panic.
func Install(L *lua.LState) error {
	protos, err := compiled()
	if err != nil {
		return err
	}

	for _, name := range Names() {
		L.PreloadModule(name, loader(name, protos[name]))
	}

	for _, name := range Names() {
		if err := warm(L, name); err != nil {
			return err
		}
	}

	return nil
}

// loader returns the package.preload entry for one module.
//
// require calls it with the module name and takes the single value it leaves
// behind as the module. The proto is instantiated here, on the state doing the
// requiring, because an LFunction belongs to the state that created it and
// must never be shared; only the proto crosses that boundary.
func loader(name string, proto *lua.FunctionProto) lua.LGFunction {
	return func(L *lua.LState) int {
		L.Push(L.NewFunctionFromProto(proto))
		L.Push(lua.LString(name))
		L.Call(1, 1)

		return 1
	}
}

// warm loads a module through require, so the result is in package.loaded
// before anything can freeze it.
//
// It goes through require rather than writing package.loaded directly so that
// the path a script takes is the path that was exercised at least once: a
// module that cannot be loaded fails the state's construction here, not on a
// player's first message.
func warm(L *lua.LState, name string) error {
	require, ok := L.GetGlobal("require").(*lua.LFunction)
	if !ok {
		return fmt.Errorf("require is missing, so the lua module %q cannot be loaded", name)
	}

	if err := L.CallByParam(lua.P{Fn: require, NRet: 1, Protect: true}, lua.LString(name)); err != nil {
		return fmt.Errorf("loading the lua module %q: %w", name, err)
	}

	loaded := L.Get(-1)
	L.Pop(1)

	if _, ok := loaded.(*lua.LTable); !ok {
		return fmt.Errorf("the lua module %q returned a %s, want a table", name, loaded.Type())
	}

	return nil
}

// compile turns one module's source into bytecode.
//
// name is the chunk name the VM reports, and it is the module name, so a
// runtime error inside a helper reads "indri.game:12: ..." -- which is what a
// script author would look for -- rather than naming a file that is not on disk
// anywhere.
func compile(name, src string) (*lua.FunctionProto, error) {
	chunk, err := parse.Parse(strings.NewReader(src), name)
	if err != nil {
		// The parse error already reads "name:line:column: message".
		return nil, fmt.Errorf("parsing the lua module: %w", err)
	}

	proto, err := lua.Compile(chunk, name)
	if err != nil {
		return nil, fmt.Errorf("compiling the lua module %q: %w", name, err)
	}

	return proto, nil
}
