package lib_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	lua "github.com/yuin/gopher-lua"

	"github.com/robbiebyrd/indri/internal/models"
	luasvc "github.com/robbiebyrd/indri/internal/services/lua"
	"github.com/robbiebyrd/indri/internal/services/lua/lib"
)

// The fixtures below are models.Game values marshalled through encoding/json,
// because that is what the helpers actually receive: the JSON view of a game,
// keyed by json tag names. Building them from the Go model rather than from a
// Lua literal means a renamed json tag breaks these tests, which is the point —
// the helpers read stage.currentScene and team.playerIds by name, and nothing
// else in this package would notice if those names moved.

// fullGame is the ordinary case: three players across two teams, a stage on a
// scene that carries data, and a second scene that does not.
func fullGame() models.Game {
	intro := map[string]interface{}{"prompt": "guess a number"}

	return models.Game{
		Code: "ABCD",
		Teams: map[string]models.Team{
			// playerIds is deliberately not in sorted order: players_in_team
			// reports the team's own order, not a sorted one.
			"red":  {Name: "Red", PlayerIDs: []string{"p2", "p1"}},
			"blue": {Name: "Blue", PlayerIDs: []string{"p3"}},
		},
		Players: map[string]models.Player{
			"p1": {Name: "One", Score: 3},
			"p2": {Name: "Two", Score: 7},
			"p3": {Name: "Three", Score: 5},
		},
		Stage: models.Stage{
			CurrentScene: "intro",
			SceneOrder:   []string{"intro", "outro"},
			Scenes: map[string]models.Scene{
				"intro": {PublicData: &intro},
				"outro": {},
			},
		},
	}
}

// teamlessGame is a game that never created a team. Teams is tagged omitempty,
// so the state the helpers see has no `teams` key at all.
func teamlessGame() models.Game {
	return models.Game{
		Players: map[string]models.Player{"p1": {Name: "One", Score: 1}},
	}
}

func TestModulesAreServedFromMemory(t *testing.T) {
	env := newEnv(t)

	preload, ok := env.L.GetField(env.L.GetGlobal("package"), "preload").(*lua.LTable)
	if !ok {
		t.Fatal("package.preload is not a table")
	}

	if len(lib.Names()) == 0 {
		t.Fatal("the library ships no modules")
	}

	for _, name := range lib.Names() {
		if got := env.L.GetField(preload, name).Type(); got != lua.LTFunction {
			t.Errorf("package.preload[%q] is a %s, want a function", name, got)
		}
	}

	// A second require must hand back the very table the first one produced:
	// the module is loaded once and cached, not rebuilt per caller.
	if again := requireModule(t, env.L); again != env.mod {
		t.Error("requiring indri.game twice returned two different tables")
	}
}

func TestCurrentScene(t *testing.T) {
	noScenes := fullGame()
	noScenes.Stage.Scenes = nil

	unknown := fullGame()
	unknown.Stage.CurrentScene = "nowhere"

	noStage := fullGame()
	noStage.Stage = models.Stage{}

	tests := []struct {
		name      string
		game      models.Game
		wantID    lua.LValue
		wantScene bool
	}{
		{"the current scene and its table", fullGame(), lua.LString("intro"), true},
		{"an id with no scene table", unknown, lua.LString("nowhere"), false},
		{"a stage with no scenes at all", noScenes, lua.LString("intro"), false},
		{"a zero stage has no current scene", noStage, lua.LNil, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := newEnv(t)

			got := env.call(t, "current_scene", 2, env.state(t, tc.game))

			if got[0] != tc.wantID {
				t.Errorf("scene id = %v, want %v", got[0], tc.wantID)
			}

			if _, isTable := got[1].(*lua.LTable); isTable != tc.wantScene {
				t.Errorf("scene table present = %t, want %t (got a %s)", isTable, tc.wantScene, got[1].Type())
			}
		})
	}
}

func TestSceneData(t *testing.T) {
	noStage := fullGame()
	noStage.Stage = models.Stage{}

	onOutro := fullGame()
	onOutro.Stage.CurrentScene = "outro"

	tests := []struct {
		name string
		game models.Game
		want map[string]string
	}{
		{"the current scene's public data", fullGame(), map[string]string{"prompt": "guess a number"}},
		{"a scene carrying no data", onOutro, map[string]string{}},
		{"no current scene", noStage, map[string]string{}},
		{"a game with nothing set", models.Game{}, map[string]string{}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := newEnv(t)

			data, ok := env.call(t, "scene_data", 1, env.state(t, tc.game))[0].(*lua.LTable)
			if !ok {
				t.Fatal("scene_data did not return a table")
			}

			if n := countKeys(data); n != len(tc.want) {
				t.Errorf("scene data holds %d keys, want %d", n, len(tc.want))
			}

			for key, want := range tc.want {
				if got := data.RawGetString(key); got.String() != want {
					t.Errorf("scene data %q = %v, want %q", key, got, want)
				}
			}
		})
	}
}

func TestTeamOf(t *testing.T) {
	tests := []struct {
		name     string
		game     models.Game
		playerID lua.LValue
		wantID   lua.LValue
		wantTeam string
	}{
		{"a player on the first team", fullGame(), lua.LString("p1"), lua.LString("red"), "Red"},
		{"a player on the second team", fullGame(), lua.LString("p3"), lua.LString("blue"), "Blue"},
		{"a player on no team", fullGame(), lua.LString("p9"), lua.LNil, ""},
		{"a game with no teams", teamlessGame(), lua.LString("p1"), lua.LNil, ""},
		{"no player id", fullGame(), lua.LNil, lua.LNil, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := newEnv(t)

			got := env.call(t, "team_of", 2, env.state(t, tc.game), tc.playerID)

			if got[0] != tc.wantID {
				t.Errorf("team id = %v, want %v", got[0], tc.wantID)
			}

			if tc.wantTeam == "" {
				return
			}

			team, ok := got[1].(*lua.LTable)
			if !ok {
				t.Fatalf("team table is a %s, want a table", got[1].Type())
			}

			if name := team.RawGetString("name").String(); name != tc.wantTeam {
				t.Errorf("team name = %q, want %q", name, tc.wantTeam)
			}
		})
	}
}

func TestPlayersInTeam(t *testing.T) {
	tests := []struct {
		name   string
		game   models.Game
		teamID lua.LValue
		want   []string
	}{
		{"members in the team's own order", fullGame(), lua.LString("red"), []string{"p2", "p1"}},
		{"a single member", fullGame(), lua.LString("blue"), []string{"p3"}},
		{"an unknown team", fullGame(), lua.LString("green"), nil},
		{"a game with no teams", teamlessGame(), lua.LString("red"), nil},
		{"no team id", fullGame(), lua.LNil, nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := newEnv(t)

			ids, ok := env.call(t, "players_in_team", 1, env.state(t, tc.game), tc.teamID)[0].(*lua.LTable)
			if !ok {
				t.Fatal("players_in_team did not return a table")
			}

			if diff := compareIDs(toStrings(ids), tc.want); diff != "" {
				t.Error(diff)
			}
		})
	}
}

// TestPlayersInTeamDoesNotAliasTheState pins the copy. Handing back the team's
// own playerIds would let a script that sorts the result edit the game while
// only meaning to read it.
func TestPlayersInTeamDoesNotAliasTheState(t *testing.T) {
	env := newEnv(t)
	game := env.state(t, fullGame())

	ids, ok := env.call(t, "players_in_team", 1, game, lua.LString("red"))[0].(*lua.LTable)
	if !ok {
		t.Fatal("players_in_team did not return a table")
	}

	ids.RawSetInt(1, lua.LString("stolen"))

	team := env.L.GetField(env.L.GetField(game, "teams"), "red")

	if got := toStrings(env.L.GetField(team, "playerIds")); len(got) == 0 || got[0] != "p2" {
		t.Errorf("the team's playerIds are now %v; the result aliases the state", got)
	}
}

func TestLeaderByScore(t *testing.T) {
	tie := fullGame()
	tie.Players = map[string]models.Player{
		"p1": {Name: "One", Score: 9},
		"p2": {Name: "Two", Score: 4},
		"p3": {Name: "Three", Score: 9},
	}

	caseTie := fullGame()
	caseTie.Players = map[string]models.Player{
		"a": {Name: "lower case", Score: 2},
		"B": {Name: "upper case", Score: 2},
	}

	allZero := fullGame()
	allZero.Players = map[string]models.Player{
		"p2": {Name: "Two"},
		"p1": {Name: "One"},
	}

	empty := fullGame()
	empty.Players = nil

	tests := []struct {
		name       string
		game       models.Game
		wantID     lua.LValue
		wantPlayer string
	}{
		{"the single highest score", fullGame(), lua.LString("p2"), "Two"},
		// The documented tie-break: equal scores go to the lowest id.
		{"a tie goes to the lowest id", tie, lua.LString("p1"), "One"},
		// Ids are compared as strings, so "B" (0x42) sorts below "a" (0x61).
		{"ids are compared as strings", caseTie, lua.LString("B"), "upper case"},
		{"every score equal", allZero, lua.LString("p1"), "One"},
		{"a game with no players", empty, lua.LNil, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := newEnv(t)

			got := env.call(t, "leader_by_score", 2, env.state(t, tc.game))

			if got[0] != tc.wantID {
				t.Errorf("leader id = %v, want %v", got[0], tc.wantID)
			}

			if tc.wantPlayer == "" {
				return
			}

			player, ok := got[1].(*lua.LTable)
			if !ok {
				t.Fatalf("leader is a %s, want a table", got[1].Type())
			}

			if name := player.RawGetString("name").String(); name != tc.wantPlayer {
				t.Errorf("leader name = %q, want %q", name, tc.wantPlayer)
			}
		})
	}
}

// TestLeaderByScoreTieIgnoresInsertionOrder pins the tie-break to the id and
// nothing else.
//
// gopher-lua walks a table's hash part in insertion order, and json.Marshal
// happens to emit map keys sorted, so a helper that simply took the first of
// pairs() would pass the tie case above by accident. Rebuilding the players
// table in each order and demanding the same answer is what rules that out.
func TestLeaderByScoreTieIgnoresInsertionOrder(t *testing.T) {
	scores := map[string]int{"p1": 9, "p2": 4, "p3": 9}

	tests := []struct {
		name  string
		order []string
	}{
		{"ascending", []string{"p1", "p2", "p3"}},
		{"descending", []string{"p3", "p2", "p1"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := newEnv(t)

			players := env.L.NewTable()

			for _, id := range tc.order {
				player := env.L.NewTable()
				player.RawSetString("name", lua.LString(id))
				player.RawSetString("score", lua.LNumber(scores[id]))
				players.RawSetString(id, player)
			}

			game := env.L.NewTable()
			game.RawSetString("players", players)

			if got := env.call(t, "leader_by_score", 2, game)[0]; got != lua.LString("p1") {
				t.Errorf("leader id = %v, want p1 whatever order the players were added in", got)
			}
		})
	}
}

func TestEachTeam(t *testing.T) {
	tests := []struct {
		name string
		game models.Game
		want []string
	}{
		{"every team, in id order", fullGame(), []string{"blue", "red"}},
		{"a game with no teams", teamlessGame(), nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := newEnv(t)

			iter, ok := env.call(t, "each_team", 1, env.state(t, tc.game))[0].(*lua.LFunction)
			if !ok {
				t.Fatal("each_team did not return an iterator function")
			}

			var ids []string

			// Bounded so a broken iterator fails the test instead of hanging it.
			for range len(tc.want) + 1 {
				pair := env.callFunction(t, iter, 2)
				if pair[0] == lua.LNil {
					break
				}

				if _, isTable := pair[1].(*lua.LTable); !isTable {
					t.Fatalf("team %v came back as a %s, want a table", pair[0], pair[1].Type())
				}

				ids = append(ids, pair[0].String())
			}

			if diff := compareIDs(ids, tc.want); diff != "" {
				t.Error(diff)
			}
		})
	}
}

// TestHelpersTolerateAnEmptyState covers the contract that every helper treats
// an absent table as an empty one. A freshly created game has no teams, no
// scenes and no data, and a helper that raised on one would take the whole
// invocation down with it.
func TestHelpersTolerateAnEmptyState(t *testing.T) {
	helpers := []string{
		"current_scene",
		"scene_data",
		"team_of",
		"players_in_team",
		"leader_by_score",
		"each_team",
	}

	for _, name := range helpers {
		t.Run(name, func(t *testing.T) {
			env := newEnv(t)

			states := []struct {
				label string
				value lua.LValue
			}{
				{"an empty table", env.L.NewTable()},
				{"no state at all", lua.LNil},
			}

			for _, tc := range states {
				// The second argument is ignored by the four helpers that take
				// one, so a single call shape covers all six.
				if _, err := env.tryCall(t, name, 1, tc.value, lua.LString("p1")); err != nil {
					t.Errorf("%s with %s: %v", name, tc.label, err)
				}
			}
		})
	}
}

// TestRequireFromAScriptChunk drives the module the way a game script does:
// from a chunk the engine loads, on a state that is sandboxed and then frozen.
// The chunk's asserts are the assertions — a failed one fails the load, and
// NewEngine returns the error.
func TestRequireFromAScriptChunk(t *testing.T) {
	engine := loadScript(t, `
local game = require("indri.game")

assert(type(game) == "table", "indri.game did not return a table")

for _, name in ipairs({"current_scene", "scene_data", "team_of",
                       "players_in_team", "leader_by_score", "each_team"}) do
    assert(type(game[name]) == "function", "indri.game." .. name .. " is missing")
end

local id = game.leader_by_score({players = {a = {score = 1}, b = {score = 2}}})

assert(id == "b", "leader_by_score returned " .. tostring(id))
`)

	defer engine.Close()
}

// TestRequireRejectsPaths is the security half of the module story. package.path
// and package.cpath are empty, so preload is the only searcher require can
// reach: a module name shaped like a path finds nothing, including the path the
// module's own source really does sit at relative to this test's directory.
func TestRequireRejectsPaths(t *testing.T) {
	onDisk, err := filepath.Abs(filepath.Join("indri", "game.lua"))
	if err != nil {
		t.Fatalf("resolving the module source path: %v", err)
	}

	if _, err := os.Stat(onDisk); err != nil {
		t.Fatalf("the module source should be readable at %s: %v", onDisk, err)
	}

	names := []string{
		"./indri/game.lua",
		"indri/game.lua",
		"indri/game",
		"../lib/indri/game.lua",
		"/etc/passwd",
		onDisk,
		"indri.game.lua",
		"indri.nope",
	}

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			engine := loadScript(t, fmt.Sprintf(`
local name = %s
local ok, err = pcall(require, name)

assert(not ok, "require(" .. name .. ") was expected to fail, but it loaded something")
assert(type(err) == "string", "require failed without an error message")
`, strconv.Quote(name)))

			defer engine.Close()
		})
	}
}

// loadScript builds an engine from src, which compiles the chunk and runs it on
// a throwaway state and again on the pool's first state.
func loadScript(t *testing.T, src string) *luasvc.Engine {
	t.Helper()

	path := filepath.Join(t.TempDir(), "script.lua")

	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatalf("writing the test script: %v", err)
	}

	engine, err := luasvc.NewEngine([]string{path}, nil)
	if err != nil {
		t.Fatalf("loading the test script: %v", err)
	}

	return engine
}

// env is one Lua state with the library installed and its module table already
// in hand. Each test case gets its own, so nothing one case leaves behind can
// reach another.
type env struct {
	L   *lua.LState
	mod *lua.LTable
}

func newEnv(t *testing.T) *env {
	t.Helper()

	L := lua.NewState()
	t.Cleanup(L.Close)

	pkg, ok := L.GetGlobal("package").(*lua.LTable)
	if !ok {
		t.Fatal("package is not a table")
	}

	// Emptied for the reason the sandbox empties them, plus one that is specific
	// to this test binary: the module's source really is on disk under the
	// working directory here, so the default package.path would let the file
	// searcher answer a require that preload is supposed to.
	pkg.RawSetString("path", lua.LString(""))
	pkg.RawSetString("cpath", lua.LString(""))

	if err := lib.Install(L); err != nil {
		t.Fatalf("installing the lua library: %v", err)
	}

	return &env{L: L, mod: requireModule(t, L)}
}

// requireModule calls require("indri.game") and returns the module table.
func requireModule(t *testing.T, L *lua.LState) *lua.LTable {
	t.Helper()

	if err := L.CallByParam(
		lua.P{Fn: L.GetGlobal("require"), NRet: 1, Protect: true},
		lua.LString("indri.game"),
	); err != nil {
		t.Fatalf("requiring indri.game: %v", err)
	}

	mod, ok := L.Get(-1).(*lua.LTable)
	L.Pop(1)

	if !ok {
		t.Fatal("indri.game did not return a table")
	}

	return mod
}

// call invokes a helper and fails the test if it raises.
func (e *env) call(t *testing.T, name string, nret int, args ...lua.LValue) []lua.LValue {
	t.Helper()

	out, err := e.tryCall(t, name, nret, args...)
	if err != nil {
		t.Fatalf("calling indri.game.%s: %v", name, err)
	}

	return out
}

// tryCall invokes a helper and hands back whatever it raised, for the cases
// that are about a helper *not* raising.
func (e *env) tryCall(t *testing.T, name string, nret int, args ...lua.LValue) ([]lua.LValue, error) {
	t.Helper()

	fn, ok := e.L.GetField(e.mod, name).(*lua.LFunction)
	if !ok {
		t.Fatalf("indri.game.%s is not a function", name)
	}

	return e.invoke(fn, nret, args...)
}

// callFunction invokes an arbitrary Lua function, such as an iterator a helper
// returned.
func (e *env) callFunction(t *testing.T, fn *lua.LFunction, nret int, args ...lua.LValue) []lua.LValue {
	t.Helper()

	out, err := e.invoke(fn, nret, args...)
	if err != nil {
		t.Fatalf("calling a lua function: %v", err)
	}

	return out
}

func (e *env) invoke(fn *lua.LFunction, nret int, args ...lua.LValue) ([]lua.LValue, error) {
	if err := e.L.CallByParam(lua.P{Fn: fn, NRet: nret, Protect: true}, args...); err != nil {
		return nil, err
	}

	// Popped back to front: the last result sits on top of the stack.
	out := make([]lua.LValue, nret)

	for i := nret - 1; i >= 0; i-- {
		out[i] = e.L.Get(-1)
		e.L.Pop(1)
	}

	return out, nil
}

// state turns a game into the table a helper receives, by the route the host
// takes: encode to JSON, then build a Lua table out of it.
func (e *env) state(t *testing.T, g models.Game) lua.LValue {
	t.Helper()

	encoded, err := json.Marshal(g)
	if err != nil {
		t.Fatalf("encoding the fixture game: %v", err)
	}

	var decoded map[string]interface{}

	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("decoding the fixture game: %v", err)
	}

	return e.toLua(t, decoded)
}

func (e *env) toLua(t *testing.T, v any) lua.LValue {
	t.Helper()

	switch v := v.(type) {
	case nil:
		return lua.LNil
	case bool:
		return lua.LBool(v)
	case float64:
		return lua.LNumber(v)
	case string:
		return lua.LString(v)
	case map[string]interface{}:
		tbl := e.L.NewTable()

		for key, item := range v {
			tbl.RawSetString(key, e.toLua(t, item))
		}

		return tbl
	case []interface{}:
		tbl := e.L.NewTable()

		for i, item := range v {
			tbl.RawSetInt(i+1, e.toLua(t, item))
		}

		return tbl
	default:
		t.Fatalf("a fixture holds a %T, which encoding/json never produces", v)

		return lua.LNil
	}
}

func toStrings(v lua.LValue) []string {
	tbl, ok := v.(*lua.LTable)
	if !ok {
		return nil
	}

	var out []string

	tbl.ForEach(func(_, item lua.LValue) {
		out = append(out, item.String())
	})

	return out
}

func countKeys(tbl *lua.LTable) int {
	n := 0

	tbl.ForEach(func(lua.LValue, lua.LValue) { n++ })

	return n
}

// compareIDs reports the difference between two id lists, or an empty string
// when they match.
func compareIDs(got, want []string) string {
	if len(got) != len(want) {
		return fmt.Sprintf("got %v, want %v", got, want)
	}

	for i := range got {
		if got[i] != want[i] {
			return fmt.Sprintf("got %v, want %v", got, want)
		}
	}

	return ""
}
