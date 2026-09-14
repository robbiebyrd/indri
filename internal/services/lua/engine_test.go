package lua

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	lua "github.com/yuin/gopher-lua"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
	"github.com/robbiebyrd/indri/internal/models"
)

// writeScript writes one script into the test's own directory and returns its
// path. Scripts are files on disk here rather than strings in memory because
// the paths are what the engine reports in every error a script author reads.
func writeScript(t *testing.T, name, src string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)

	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatalf("writing %v: %v", path, err)
	}

	return path
}

// writeScripts writes several scripts into one directory, in the order given,
// and returns their paths in that order.
func writeScripts(t *testing.T, sources map[string]string, order ...string) []string {
	t.Helper()

	dir := t.TempDir()
	paths := make([]string, 0, len(order))

	for _, name := range order {
		path := filepath.Join(dir, name)

		if err := os.WriteFile(path, []byte(sources[name]), 0o600); err != nil {
			t.Fatalf("writing %v: %v", path, err)
		}

		paths = append(paths, path)
	}

	return paths
}

// newTestEngine builds an engine with no game store and closes it when the test
// ends. Its scripts load and run; indri.mutate raises. Tests that need a real
// store want newTestEngineWithGames.
func newTestEngine(t *testing.T, paths ...string) *Engine {
	t.Helper()

	return newTestEngineWithGames(t, nil, paths...)
}

// newTestEngineWithGames builds an engine over games and closes it when the
// test ends.
func newTestEngineWithGames(t *testing.T, games GameMutator, paths ...string) *Engine {
	t.Helper()

	e, err := NewEngine(paths, games)
	if err != nil {
		t.Fatalf("building an engine from %v: %v", paths, err)
	}

	t.Cleanup(e.Close)

	return e
}

// loadOn runs chunks on a freshly built state, the way the pool's prepare hook
// does, and returns the state with its handlers.
func loadOn(t *testing.T, chunks []scriptChunk) (*lua.LState, *stateHandlers) {
	t.Helper()

	L, err := defaultState()
	if err != nil {
		t.Fatalf("building a state: %v", err)
	}

	t.Cleanup(L.Close)

	h, err := installScripts(L, chunks)
	if err != nil {
		t.Fatalf("loading scripts: %v", err)
	}

	return L, h
}

// mustCompileFiles compiles scripts the test asserts are valid.
func mustCompileFiles(t *testing.T, paths ...string) []scriptChunk {
	t.Helper()

	chunks, err := compileScripts(paths)
	if err != nil {
		t.Fatalf("compiling %v: %v", paths, err)
	}

	return chunks
}

// splitScriptError separates the models.WSError frame Invoke answers a failed
// script with from the frames the script itself replied with.
//
// Invoke does not report a script's own failure through its error return — a
// WebSocket player would never see it, so the failure is packed into Responses
// instead and only a host failure reaches the error return. Tests whose subject
// is something else fold that frame back into an ordinary error through this,
// rather than each unpacking it by hand. The packing itself is asserted in
// host_io_test.go, and on all three transports in
// internal/services/boot/handlers_test.go.
func splitScriptError(res actions.Result, err error) (actions.Result, error) {
	if err != nil {
		return res, err
	}

	kept := make([][]byte, 0, len(res.Responses))

	var failure error

	for _, response := range res.Responses {
		var frame models.WSError

		// The code is what tells a script's failure from a reply: a reply is an
		// arbitrary JSON object, and every object unmarshals into a WSError with
		// a zero code.
		if json.Unmarshal(response, &frame) == nil && frame.ErrorCode == models.ErrScriptFailed.ErrorCode {
			failure = errors.New(frame.Message)

			continue
		}

		kept = append(kept, response)
	}

	res.Responses = kept

	return res, failure
}

// requireErrorMentions fails unless err is non-nil and names every want.
func requireErrorMentions(t *testing.T, err error, wants ...string) {
	t.Helper()

	if err == nil {
		t.Fatalf("expected an error mentioning %v, got none", wants)
	}

	for _, want := range wants {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not mention %q", err.Error(), want)
		}
	}
}

// callHandler calls one of a state's handlers and returns its numeric result.
func callHandler(t *testing.T, L *lua.LState, fn *lua.LFunction) float64 {
	t.Helper()

	if err := L.CallByParam(lua.P{Fn: fn, NRet: 1, Protect: true}); err != nil {
		t.Fatalf("calling a handler: %v", err)
	}

	ret := L.Get(-1)
	L.Pop(1)

	n, ok := ret.(lua.LNumber)
	if !ok {
		t.Fatalf("a handler returned %s, want a number", ret.Type())
	}

	return float64(n)
}

// TestNewEngine_DeclaresEveryRegisteredAction is the manifest in its ordinary
// form: several files, several registrations, one sorted list.
func TestNewEngine_DeclaresEveryRegisteredAction(t *testing.T) {
	t.Parallel()

	paths := writeScripts(t, map[string]string{
		"moves.lua": `
indri.on("move", function(req) return req end)
indri.on("undo", function(req) return req end)
`,
		"chat.lua": `
indri.on("say", function(req) return req end)
`,
	}, "moves.lua", "chat.lua")

	e := newTestEngine(t, paths...)

	want := []string{"move", "say", "undo"}
	if got := e.Actions(); !slices.Equal(got, want) {
		t.Fatalf("Actions() = %v, want %v", got, want)
	}
}

// TestEngine_ActionsCannotBeMutatedByACaller guards the "frozen manifest" half
// of the design: the router must not be able to edit what boot decided.
func TestEngine_ActionsCannotBeMutatedByACaller(t *testing.T) {
	t.Parallel()

	e := newTestEngine(t, writeScript(t, "game.lua", `indri.on("move", function() end)`))

	e.Actions()[0] = "tampered"

	if got := e.Actions(); !slices.Equal(got, []string{"move"}) {
		t.Fatalf("Actions() = %v after a caller wrote to the returned slice, want [move]", got)
	}
}

// TestNewEngine_AcceptsNoScripts: a server with no game script still boots, and
// simply declares nothing.
func TestNewEngine_AcceptsNoScripts(t *testing.T) {
	t.Parallel()

	if got := newTestEngine(t).Actions(); len(got) != 0 {
		t.Fatalf("Actions() = %v with no scripts loaded, want none", got)
	}
}

// TestNewEngine_RefusesADuplicateAction covers both halves of the rule. Two
// registrations in one file are the obvious case; two files claiming the same
// action are the case a per-file check would miss, and the one an operator
// actually hits when they copy a script.
func TestNewEngine_RefusesADuplicateAction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		sources map[string]string
		order   []string
		wants   []string
	}{
		{
			name: "twice in one file",
			sources: map[string]string{"game.lua": `
indri.on("move", function() end)
indri.on("move", function() end)
`},
			order: []string{"game.lua"},
			wants: []string{"game.lua", `"move"`, "already registered", "game.lua:2"},
		},
		{
			name: "once in each of two files",
			sources: map[string]string{
				"first.lua":  `indri.on("move", function() end)`,
				"second.lua": `indri.on("move", function() end)`,
			},
			order: []string{"first.lua", "second.lua"},
			wants: []string{"second.lua", `"move"`, "already registered", "first.lua:1"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := NewEngine(writeScripts(t, test.sources, test.order...), nil)
			requireErrorMentions(t, err, test.wants...)
		})
	}
}

// TestNewEngine_ReportsASyntaxErrorWithItsFileAndLine: a script author's first
// question about a failed boot is which file and which line.
func TestNewEngine_ReportsASyntaxErrorWithItsFileAndLine(t *testing.T) {
	t.Parallel()

	path := writeScript(t, "broken.lua", `
local ok = true
local 1 = 2
return ok
`)

	_, err := NewEngine([]string{path}, nil)
	requireErrorMentions(t, err, path, ":3")
}

// TestNewEngine_ReportsARuntimeErrorWithItsFile: a chunk that fails while it is
// running is a load failure too, and names the file it came from.
func TestNewEngine_ReportsARuntimeErrorWithItsFile(t *testing.T) {
	t.Parallel()

	path := writeScript(t, "angry.lua", `
indri.on("move", function() end)
error("no")
`)

	_, err := NewEngine([]string{path}, nil)
	requireErrorMentions(t, err, path, ":3", "no")
}

// TestNewEngine_ReportsAMissingFile names the path that is not there, rather
// than failing somewhere deeper with a bare "no such file".
func TestNewEngine_ReportsAMissingFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "absent.lua")

	_, err := NewEngine([]string{path}, nil)
	requireErrorMentions(t, err, path, "reading lua script")
}

// TestNewEngine_RefusesTheSameFileTwice: listing a file twice would otherwise
// surface as a duplicate-action error blaming the script for the config's
// mistake.
func TestNewEngine_RefusesTheSameFileTwice(t *testing.T) {
	t.Parallel()

	path := writeScript(t, "game.lua", `indri.on("move", function() end)`)

	_, err := NewEngine([]string{path, path}, nil)
	requireErrorMentions(t, err, path, "listed more than once")
}

// TestNewEngine_RefusesAReservedAction runs every reserved name through a real
// load. received and processed run on every message; the built-ins own the
// authentication and membership surface, and registration is additive, so a
// script claiming "login" would run alongside the real one on a payload
// carrying a plaintext password.
func TestNewEngine_RefusesAReservedAction(t *testing.T) {
	t.Parallel()

	reserved := slices.Concat(dispatchPhases, builtinActions)

	for _, action := range reserved {
		t.Run(action, func(t *testing.T) {
			t.Parallel()

			path := writeScript(t, "game.lua", `indri.on("`+action+`", function() end)`)

			_, err := NewEngine([]string{path}, nil)
			requireErrorMentions(t, err, "game.lua:1", `"`+action+`"`, "reserved")
		})
	}
}

// TestNewEngine_RefusesAnEmptyAction: an unnamed action is unreachable, so
// accepting it would silently swallow the handler.
func TestNewEngine_RefusesAnEmptyAction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		action string
	}{
		{name: "empty", action: ""},
		{name: "whitespace", action: "   "},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			path := writeScript(t, "game.lua", `indri.on("`+test.action+`", function() end)`)

			_, err := NewEngine([]string{path}, nil)
			requireErrorMentions(t, err, "game.lua:1", "cannot be empty")
		})
	}
}

// scriptDispatcherPkg is the one directory under handlers/actions that is not a
// built-in action: it holds the handler that runs a *script's* actions, one
// instance per entry in Engine.Actions(). Reserving its name would stop a
// script declaring an action called "script" for no reason at all.
const scriptDispatcherPkg = "script"

// TestBuiltinActions_CoversEveryActionPackage keeps the reserved list honest.
// The list cannot be read from boot.registerHandlers — boot depends on this
// package — so it is checked against the same directory that
// TestRegisterHandlers_CoversEveryActionPackage holds boot to.
func TestBuiltinActions_CoversEveryActionPackage(t *testing.T) {
	t.Parallel()

	const actionsDir = "../../handlers/actions"

	entries, err := os.ReadDir(actionsDir)
	if err != nil {
		t.Fatalf("reading %v: %v", actionsDir, err)
	}

	found := 0

	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == scriptDispatcherPkg {
			continue
		}

		found++

		if !slices.Contains(builtinActions, entry.Name()) {
			t.Errorf("the built-in action %q is not reserved, so a script could shadow it", entry.Name())
		}
	}

	if found == 0 {
		t.Fatalf("found no action packages under %v", actionsDir)
	}

	for _, action := range builtinActions {
		if _, err := os.Stat(filepath.Join(actionsDir, action)); err != nil {
			t.Errorf("the reserved action %q has no handler package: %v", action, err)
		}
	}
}

// TestNewEngine_FailsWhenStatesDisagreeAboutTheirActions is the boot-loudly
// rule. A script naming an action after something that varies — here a random
// number — gives every state a different action set, which would make reaching
// an action depend on which pooled state a player's message landed on.
func TestNewEngine_FailsWhenStatesDisagreeAboutTheirActions(t *testing.T) {
	t.Parallel()

	path := writeScript(t, "unstable.lua", `
indri.on("move_" .. tostring(math.random()), function() end)
`)

	_, err := NewEngine([]string{path}, nil)
	requireErrorMentions(t, err, "not deterministic", "move_")
}

// TestStateHandlers_AgreesWith states the comparison the boot check rests on,
// without depending on a source of non-determinism.
func TestStateHandlers_AgreesWith(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		registered []string
		subscribed []string
		manifest   []string
		lifecycle  []string
		wantErr    bool
	}{
		{name: "same actions", registered: []string{"move", "say"}, manifest: []string{"move", "say"}},
		{name: "none at all", registered: nil, manifest: nil},
		{name: "one missing", registered: []string{"move"}, manifest: []string{"move", "say"}, wantErr: true},
		{name: "one extra", registered: []string{"move", "say", "undo"}, manifest: []string{"move", "say"}, wantErr: true},
		{name: "different name", registered: []string{"jump"}, manifest: []string{"move"}, wantErr: true},
		{
			name:       "same lifecycle events",
			subscribed: []string{LifecyclePlayerJoined},
			lifecycle:  []string{LifecyclePlayerJoined},
		},
		{
			// The namespaces are compared separately, so a state that subscribed
			// to nothing must not be excused by having registered the actions.
			name:       "lifecycle event missing",
			registered: []string{"move"},
			manifest:   []string{"move"},
			lifecycle:  []string{LifecyclePlayerLeft},
			wantErr:    true,
		},
		{
			name:       "lifecycle event extra",
			subscribed: []string{LifecycleGameCreated, LifecycleSceneChanged},
			lifecycle:  []string{LifecycleGameCreated},
			wantErr:    true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			h := newStateHandlers()
			for _, action := range test.registered {
				h.fns[action] = &lua.LFunction{}
			}

			for _, event := range test.subscribed {
				h.lifecycle[event] = &lua.LFunction{}
			}

			err := h.agreesWith(test.manifest, test.lifecycle)
			if gotErr := err != nil; gotErr != test.wantErr {
				t.Fatalf("agreesWith(%v, %v) with %v/%v registered returned %v, wantErr %v",
					test.manifest, test.lifecycle, test.registered, test.subscribed, err, test.wantErr)
			}
		})
	}
}

// TestInstallScripts_GivesEachStateItsOwnHandlers is the reason the engine does
// not keep a handler map of its own. The closure below captures an upvalue; if
// two states shared one *lua.LFunction they would share that counter, and the
// second state's first call would return 3.
func TestInstallScripts_GivesEachStateItsOwnHandlers(t *testing.T) {
	t.Parallel()

	chunks := mustCompileFiles(t, writeScript(t, "counter.lua", `
local calls = 0
indri.on("move", function()
  calls = calls + 1
  return calls
end)
`))

	first, firstHandlers := loadOn(t, chunks)
	second, secondHandlers := loadOn(t, chunks)

	firstFn, ok := firstHandlers.lookup("move")
	if !ok {
		t.Fatal(`the first state did not register "move"`)
	}

	secondFn, ok := secondHandlers.lookup("move")
	if !ok {
		t.Fatal(`the second state did not register "move"`)
	}

	if firstFn == secondFn {
		t.Fatal("two states share one handler function; a closure cannot cross states")
	}

	for want := 1.0; want <= 2; want++ {
		if got := callHandler(t, first, firstFn); got != want {
			t.Fatalf("the first state's handler returned %v, want %v", got, want)
		}
	}

	if got := callHandler(t, second, secondFn); got != 1 {
		t.Fatalf("the second state's handler returned %v, want 1: its upvalue is not its own", got)
	}
}

// TestEngine_EveryPooledStateRunsTheChunksItself checks the same thing through
// the pool the engine actually serves from: each state carries its own handler
// map, and every map matches the manifest.
func TestEngine_EveryPooledStateRunsTheChunksItself(t *testing.T) {
	t.Parallel()

	e := newTestEngine(t, writeScript(t, "game.lua", `
indri.on("move", function() end)
indri.on("say", function() end)
`))

	const states = 3

	var (
		held []*pooledState
		fns  []*lua.LFunction
	)

	for range states {
		s, err := e.pool.acquire()
		if err != nil {
			t.Fatalf("acquiring a state: %v", err)
		}

		held = append(held, s)

		h, err := handlersFor(s.L)
		if err != nil {
			t.Fatalf("reading a state's handlers: %v", err)
		}

		if got := h.names(); !slices.Equal(got, e.Actions()) {
			t.Fatalf("a pooled state registered %v, the manifest says %v", got, e.Actions())
		}

		fn, ok := h.lookup("move")
		if !ok {
			t.Fatal(`a pooled state did not register "move"`)
		}

		fns = append(fns, fn)
	}

	for _, s := range held {
		e.pool.release(s)
	}

	for i := range fns {
		for j := i + 1; j < len(fns); j++ {
			if fns[i] == fns[j] {
				t.Fatal("two pooled states share one handler function")
			}
		}
	}
}

// TestInstallScripts_RefusesRegistrationAfterLoad closes the window a handler
// could otherwise use to register a new action mid-game, when the manifest the
// router was built from is already fixed.
func TestInstallScripts_RefusesRegistrationAfterLoad(t *testing.T) {
	t.Parallel()

	L, _ := loadOn(t, mustCompileFiles(t, writeScript(t, "game.lua", `
indri.on("move", function() end)
`)))

	err := L.DoString(`indri.on("late", function() end)`)
	requireErrorMentions(t, err, `"late"`, "while a script is loading")
}

// TestInstallScripts_FreezesTheHostTable: the indri table is shared by every
// invocation that runs on a pooled state, so one invocation must not be able to
// rewrite it for the next.
func TestInstallScripts_FreezesTheHostTable(t *testing.T) {
	t.Parallel()

	L, _ := loadOn(t, mustCompileFiles(t, writeScript(t, "game.lua", `
indri.on("move", function() end)
`)))

	tests := []struct {
		name string
		src  string
	}{
		{name: "replace on", src: `indri.on = function() end`},
		{name: "remove on", src: `indri.on = nil`},
		{name: "add a field", src: `indri.evil = true`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requireErrorMentions(t, L.DoString(test.src), "read-only")
		})
	}
}

// TestHandlersFor_ReportsAnUnpreparedState: the lookup is how every later
// caller reaches a state's handlers, so its failure has to be legible rather
// than a nil map panic.
func TestHandlersFor_ReportsAnUnpreparedState(t *testing.T) {
	t.Parallel()

	L, err := defaultState()
	if err != nil {
		t.Fatalf("building a state: %v", err)
	}

	t.Cleanup(L.Close)

	if _, err := handlersFor(L); err == nil {
		t.Fatal("handlersFor succeeded on a state that never loaded a script")
	}
}
