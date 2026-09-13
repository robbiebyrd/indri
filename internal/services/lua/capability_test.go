package lua

import (
	"context"
	"slices"
	"sync"
	"testing"

	lua "github.com/yuin/gopher-lua"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
	"github.com/robbiebyrd/indri/internal/models"
)

// probeName is the capability these tests inject.
//
// It is a test capability rather than one this build ships because the point
// being proved is the mechanism, not any particular capability: a test written
// against http would stop testing injection the day http grew a network of its
// own to go wrong.
const probeName = "probe"

// probeLog records every call a granted script made, so a test can tell "the
// script reached the capability" from "the script ran and did nothing".
type probeLog struct {
	mu    sync.Mutex
	marks []string
}

func (p *probeLog) add(mark string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.marks = append(p.marks, mark)
}

func (p *probeLog) all() []string {
	p.mu.Lock()
	defer p.mu.Unlock()

	return slices.Clone(p.marks)
}

// probeCapability installs indri.probe, a table with one function that reports
// what it was called with back to Go.
func probeCapability(calls *probeLog) capabilityInstaller {
	return func(L *lua.LState) (lua.LValue, error) {
		tbl := L.NewTable()

		tbl.RawSetString("mark", L.NewFunction(func(L *lua.LState) int {
			calls.add(L.CheckString(1))

			return 0
		}))

		return tbl, nil
	}
}

// probeSet is the capability set these tests build engines against.
func probeSet(calls *probeLog) capabilitySet {
	return capabilitySet{probeName: probeCapability(calls)}
}

// newProbeEngine builds an engine over the given scripts, with probe as the
// only capability that exists, and closes it when the test ends.
func newProbeEngine(t *testing.T, calls *probeLog, scripts ...models.ScriptFile) *Engine {
	t.Helper()

	e, err := newEngine(scripts, nil, probeSet(calls))
	if err != nil {
		t.Fatalf("building an engine from %+v: %v", scripts, err)
	}

	t.Cleanup(e.Close)

	return e
}

// granted pairs a path with the capabilities its config entry hands it.
func granted(path string, grants ...string) models.ScriptFile {
	return models.ScriptFile{Path: path, Grants: grants}
}

// reachScript is the ungranted half of the isolation tests. It asserts that
// every route a script could take to a capability it was not given comes back
// empty, and raises the name of the route that did not.
//
// The two pairs() walks are worth stating even though a frozen table yields
// nothing: that emptiness is what freezeTable buys, and a future change that
// froze the globals differently would show up here rather than in a security
// report.
const reachScript = `
indri.on("reach", function(req)
  assert(indri.probe == nil, "indri.probe")
  assert(_G.indri.probe == nil, "_G.indri.probe")
  assert(_G.probe == nil, "_G.probe")
  assert(package.loaded.probe == nil, "package.loaded.probe")
  assert(package.loaded["indri.probe"] == nil, "package.loaded['indri.probe']")
  assert(package.preload.probe == nil, "package.preload.probe")

  -- The metatable route: the sandbox leaves a script no way to ask for one, so
  -- the backing table freezeTable hides behind __index cannot be named.
  assert(getmetatable == nil, "getmetatable")
  assert(rawget == nil, "rawget")

  for key in pairs(_G) do
    assert(key ~= "probe", "_G walk found probe")
  end

  for key in pairs(package.loaded) do
    assert(key ~= "probe", "package.loaded walk found probe")
  end
end)
`

// markScript is the granted half: it uses the capability at load time and again
// from inside its handler, because those are two different lookups — the chunk
// reads the scoped table through its own global, the handler reads it through
// the environment the host installs for the call.
const markScript = `
indri.probe.mark("at-load")

indri.on("mark", function(req)
  indri.probe.mark("at-invoke:" .. tostring(req.payload.word))
end)
`

// invokeOK runs an action and fails the test if the script raised.
func invokeOK(t *testing.T, e *Engine, action string, payload map[string]interface{}) {
	t.Helper()

	// Through splitScriptError, because these scripts report by asserting: a
	// failed assert is now a frame in Responses rather than an error return, so
	// checking only the error would pass with every assertion in them failing.
	if _, err := splitScriptError(e.Invoke(context.Background(), action, actions.Request{Payload: payload})); err != nil {
		t.Fatalf("invoking %q: %v", action, err)
	}
}

// TestCapabilities_AreAbsentWithoutAGrant is the property the whole mechanism
// exists for: a script that was not granted a capability does not see a
// function that checks permissions, it sees nothing at all.
//
// Both scripts are loaded into one engine, so this is also the per-script
// claim: the grant on the first is invisible to the second even though they
// share a process, an engine and every pooled state.
func TestCapabilities_AreAbsentWithoutAGrant(t *testing.T) {
	t.Parallel()

	calls := &probeLog{}
	paths := writeScripts(t,
		map[string]string{"mark.lua": markScript, "reach.lua": reachScript},
		"mark.lua", "reach.lua",
	)

	e := newProbeEngine(t, calls, granted(paths[0], probeName), granted(paths[1]))

	invokeOK(t, e, "reach", nil)
	invokeOK(t, e, "mark", map[string]interface{}{"word": "hello"})

	if !slices.Contains(calls.all(), "at-invoke:hello") {
		t.Fatalf("the granted script's handler recorded %v, want an at-invoke mark", calls.all())
	}
}

// TestCapabilities_ReachTheGrantedScriptAtLoadAndAtInvoke covers the lookup a
// naive implementation gets wrong. A capability installed only while the chunk
// runs is nil by the time a player triggers the handler, because every
// invocation is given a fresh environment whose fallback is the shared host
// table.
func TestCapabilities_ReachTheGrantedScriptAtLoadAndAtInvoke(t *testing.T) {
	t.Parallel()

	calls := &probeLog{}
	path := writeScript(t, "mark.lua", markScript)

	e := newProbeEngine(t, calls, granted(path, probeName))

	invokeOK(t, e, "mark", map[string]interface{}{"word": "one"})

	got := calls.all()

	for _, want := range []string{"at-load", "at-invoke:one"} {
		if !slices.Contains(got, want) {
			t.Fatalf("the probe recorded %v, want it to contain %q", got, want)
		}
	}
}

// TestCapabilities_StillCarryTheSharedHostAPI guards the copy scopeHostTable
// makes: a scoped table that dropped indri.on or indri.mutate would break every
// granted script in a way no capability test would notice.
func TestCapabilities_StillCarryTheSharedHostAPI(t *testing.T) {
	t.Parallel()

	calls := &probeLog{}
	path := writeScript(t, "host.lua", `
assert(type(indri.on) == "function", "indri.on is missing from the scoped table")
assert(type(indri.mutate) == "function", "indri.mutate is missing from the scoped table")

indri.on("host", function(req)
  assert(type(indri.on) == "function", "indri.on is missing at invoke")
  assert(type(indri.mutate) == "function", "indri.mutate is missing at invoke")
  assert(type(indri.probe.mark) == "function", "indri.probe.mark is missing at invoke")
end)
`)

	e := newProbeEngine(t, calls, granted(path, probeName))

	invokeOK(t, e, "host", nil)
}

// TestCapabilities_AreReadOnly keeps one invocation from editing the table the
// next one on the same pooled state will read. The scoped table outlives the
// call: it belongs to the state.
func TestCapabilities_AreReadOnly(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"replacing the capability table":  `indri.probe = nil`,
		"replacing a capability function": `indri.probe.mark = function() end`,
		"adding to the capability table":  `indri.probe.extra = 1`,
	}

	for name, attempt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			calls := &probeLog{}
			path := writeScript(t, "write.lua", `
indri.on("write", function(req)
  `+attempt+`
end)
`)

			e := newProbeEngine(t, calls, granted(path, probeName))

			_, err := splitScriptError(e.Invoke(context.Background(), "write", actions.Request{}))
			requireErrorMentions(t, err, "read-only")
		})
	}
}

// TestCapabilities_DoNotBreakPerInvocationGlobals re-proves the pool's
// isolation for a granted script, because a granted handler is wrapped and the
// wrapper is what builds its environment. A counter surviving between calls
// here would mean one player's move leaked into the next.
func TestCapabilities_DoNotBreakPerInvocationGlobals(t *testing.T) {
	t.Parallel()

	calls := &probeLog{}
	path := writeScript(t, "count.lua", `
indri.on("count", function(req)
  counter = (counter or 0) + 1
  indri.probe.mark("counter=" .. counter)
end)
`)

	e := newProbeEngine(t, calls, granted(path, probeName))

	for range 2 {
		invokeOK(t, e, "count", nil)
	}

	for _, mark := range calls.all() {
		if mark != "counter=1" {
			t.Fatalf("the probe recorded %v, want every call to start from a fresh globals table", calls.all())
		}
	}
}

// TestNewEngineWithGrants_RefusesABadGrantList is the boot-time half. Every one
// of these would otherwise be invisible until a player triggered the handler
// that expected the capability, and would look to the script author like the
// capability simply does not work.
func TestNewEngineWithGrants_RefusesABadGrantList(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		grants []string
		path   string
		wants  []string
	}{
		"a misspelled capability": {
			grants: []string{"prbe"},
			wants:  []string{"unknown capability", `"prbe"`, probeName},
		},
		"a name no capability in this set answers to": {
			grants: []string{CapabilityHTTP},
			wants:  []string{"unknown capability", CapabilityHTTP},
		},
		"the same grant twice": {
			grants: []string{probeName, probeName},
			wants:  []string{"more than once", probeName},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := writeScript(t, "game.lua", `indri.on("noop", function(req) end)`)

			_, err := newEngine(
				[]models.ScriptFile{granted(path, test.grants...)},
				nil,
				probeSet(&probeLog{}),
			)

			requireErrorMentions(t, err, append(test.wants, path)...)
		})
	}
}

// TestNewEngineWithGrants_RefusesAnEntryWithNoPath keeps a mistyped config from
// being read as "load nothing and carry on".
func TestNewEngineWithGrants_RefusesAnEntryWithNoPath(t *testing.T) {
	t.Parallel()

	for name, path := range map[string]string{"empty": "", "blank": "   "} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := NewEngineWithGrants([]models.ScriptFile{{Path: path}}, nil)
			requireErrorMentions(t, err, "has no path")
		})
	}
}

// TestNewEngineWithGrants_RefusesACapabilityThisBuildHasNotImplemented is the
// difference between a typo and a name this server understands but cannot
// serve. Both fail boot; only one of them is the operator's fault, and the
// messages have to say which.
//
// It is written against a reserved name of its own rather than against a
// shipped one, because every capability this build ships now has an installer:
// a test that named one would stop testing the reserved-but-unbuilt path the
// day that name was implemented, which is exactly what happened to http and
// assets.
func TestNewEngineWithGrants_RefusesACapabilityThisBuildHasNotImplemented(t *testing.T) {
	t.Parallel()

	const reserved = "reserved"

	path := writeScript(t, "game.lua", `indri.on("noop", function(req) end)`)

	_, err := newEngine(
		[]models.ScriptFile{granted(path, reserved)},
		nil,
		capabilitySet{reserved: nil},
	)

	requireErrorMentions(t, err, "does not implement yet", reserved, path)
}

// TestDefaultCapabilities_AreTheNamesTheGrantListAccepts pins the vocabulary a
// config may use. A name in the set without an installer is a name an operator
// can write and a script can never use, so the two halves are checked together
// here.
func TestDefaultCapabilities_AreTheNamesTheGrantListAccepts(t *testing.T) {
	t.Parallel()

	want := []string{CapabilityAssets, CapabilityHTTP}

	if got := defaultCapabilities.names(); !slices.Equal(got, want) {
		t.Fatalf("the known capabilities are %v, want %v", got, want)
	}

	for name, install := range defaultCapabilities {
		if install == nil {
			t.Errorf("the capability %q has no installer, so granting it would fail boot", name)
		}
	}
}

// TestNewEngine_GrantsNothing is the contract the no-grants constructor keeps,
// and the reason every existing script still behaves exactly as it did: an
// ungranted script shares the host table rather than getting a copy of it.
func TestNewEngine_GrantsNothing(t *testing.T) {
	t.Parallel()

	paths := writeScripts(t, map[string]string{"reach.lua": reachScript}, "reach.lua")

	e, err := NewEngine(paths, nil)
	if err != nil {
		t.Fatalf("building an engine from %v: %v", paths, err)
	}

	t.Cleanup(e.Close)

	invokeOK(t, e, "reach", nil)
}
