package lua

import (
	"fmt"
	"strings"
	"sync"

	lua "github.com/yuin/gopher-lua"
	"github.com/yuin/gopher-lua/parse"
)

// compile turns Lua source into bytecode.
//
// The distinction this package is built around lives here. A
// *lua.FunctionProto is immutable and holds no reference to the state that
// produced it, so one proto is safe to share across every pooled state and
// every goroutine — compile once at boot and hand the same pointer out
// forever. A *lua.LFunction is the opposite: NewFunctionFromProto binds the
// new function to the creating state's environment and upvalues, so it must
// never leave the state that made it. Anything crossing a state boundary is a
// proto; anything that cannot is an LFunction.
//
// name is the chunk name the VM reports in errors and stack traces, so it
// should be the script's path.
func compile(name, src string) (*lua.FunctionProto, error) {
	chunk, err := parse.Parse(strings.NewReader(src), name)
	if err != nil {
		// The parse error already reads "name:line:column: message", so the
		// chunk name is deliberately not repeated here.
		return nil, fmt.Errorf("parsing lua source: %w", err)
	}

	proto, err := lua.Compile(chunk, name)
	if err != nil {
		return nil, fmt.Errorf("compiling %s: %w", name, err)
	}

	return proto, nil
}

// protoCache compiles each chunk once and hands the same bytecode to everyone
// who asks for it afterwards.
//
// It is keyed by chunk name — a script path — and remembers the source it
// compiled. Identical source served from the cache is the point; *changed*
// source under a name already in the cache compiles again, so a reload can
// never be answered with stale bytecode.
//
// The mutex is held across the compile so a chunk asked for by two goroutines
// at once is compiled once rather than twice. Compilation happens at boot, not
// per request, so serialising it costs nothing.
type protoCache struct {
	mu      sync.Mutex
	entries map[string]cachedProto
}

type cachedProto struct {
	src   string
	proto *lua.FunctionProto
}

func newProtoCache() *protoCache {
	return &protoCache{entries: make(map[string]cachedProto)}
}

// compile returns the bytecode for src, compiling it only if this exact source
// has not already been compiled under this name.
func (c *protoCache) compile(name, src string) (*lua.FunctionProto, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, ok := c.entries[name]; ok && entry.src == src {
		return entry.proto, nil
	}

	proto, err := compile(name, src)
	if err != nil {
		return nil, err
	}

	c.entries[name] = cachedProto{src: src, proto: proto}

	return proto, nil
}
