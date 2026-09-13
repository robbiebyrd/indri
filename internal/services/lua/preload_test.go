package lua

import (
	"testing"

	lua "github.com/yuin/gopher-lua"
)

// TestRequireSurvivesTheFreeze is the reason lib.Install requires each module
// as well as preloading it.
//
// freezeState makes package.loaded read-only, and require records a first load
// with a write to that table — so a module not already loaded can never be
// loaded again on that state. A module the sandbox loaded while the state was
// still writable is a cached read instead, which a frozen table still serves
// through its __index. This test is what keeps those two halves together: drop
// the warming half and it fails here rather than on a player's first message.
func TestRequireSurvivesTheFreeze(t *testing.T) {
	L, err := defaultState()
	if err != nil {
		t.Fatalf("building a sandboxed state: %v", err)
	}

	defer L.Close()

	if err := freezeState(L); err != nil {
		t.Fatalf("freezing the state: %v", err)
	}

	tests := []struct {
		name   string
		module string
		wantOK bool
	}{
		{"a module the sandbox loaded", "indri.game", true},
		// The counterpart, and the reason every module has to ship warmed: the
		// preload searcher would find an unloaded module, run it, and then fail
		// trying to cache it.
		{"a module nothing loaded", "indri.missing", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := L.CallByParam(
				lua.P{Fn: L.GetGlobal("require"), NRet: 1, Protect: true},
				lua.LString(tc.module),
			)

			if tc.wantOK != (err == nil) {
				t.Fatalf("require(%q) returned %v, want ok = %t", tc.module, err, tc.wantOK)
			}

			if err != nil {
				return
			}

			mod := L.Get(-1)
			L.Pop(1)

			if _, ok := mod.(*lua.LTable); !ok {
				t.Errorf("require(%q) returned a %s, want a table", tc.module, mod.Type())
			}
		})
	}
}
