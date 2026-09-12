package lua

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	lua "github.com/yuin/gopher-lua"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/robbiebyrd/indri/internal/models"
)

const testGameID = "507f1f77bcf86cd799439011"

// fullGame builds a game with every field populated and every collection
// non-empty. A fresh instance is returned on each call so a test can compare an
// edited game against a pristine one without copying by hand.
//
// Numbers inside the *Data maps are float64 on purpose: those maps are
// interface{}-valued, and a game loaded from Mongo and rendered by
// events.ToMap holds float64 there. Writing int here would make the round trip
// look lossy when it is not.
func fullGame(t *testing.T) *models.Game {
	t.Helper()

	id, err := bson.ObjectIDFromHex(testGameID)
	if err != nil {
		t.Fatalf("parsing the fixture object id: %v", err)
	}

	return &models.Game{
		ID:        id,
		Version:   7,
		CreatedAt: time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 9, 12, 11, 30, 0, 123456789, time.UTC),
		DeletedAt: time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC),
		Code:      "ABCD",
		Private:   true,
		Teams: map[string]models.Team{
			"red": {
				Name:        "Red",
				PlayerIDs:   []string{"alice"},
				PublicData:  map[string]interface{}{"turn": true},
				PrivateData: map[string]interface{}{"seed": float64(12)},
				PlayerData:  map[string]map[string]interface{}{"alice": {"ready": true}},
			},
			"blue": {
				Name:       "Blue",
				PlayerIDs:  []string{"bob"},
				PublicData: map[string]interface{}{"turn": false},
			},
		},
		Players: map[string]models.Player{
			"alice": {
				Name:        "Alice",
				Score:       3,
				Connected:   true,
				Host:        true,
				PublicData:  &map[string]interface{}{"colour": "red"},
				PrivateData: &map[string]interface{}{"hand": []interface{}{"a", "b"}},
			},
			"bob": {
				Name:       "Bob",
				Score:      1,
				Controller: true,
				PublicData: &map[string]interface{}{"colour": "blue"},
			},
		},
		Stage: models.Stage{
			CurrentScene: "board",
			SceneOrder:   []string{"lobby", "board", "results"},
			Scenes: map[string]models.Scene{
				"board": {
					PublicData:  &map[string]interface{}{"cells": []interface{}{"x", "", "o"}},
					PrivateData: &map[string]interface{}{"solution": "x"},
					PlayerData:  &map[string]map[string]interface{}{"alice": {"seen": true}},
				},
				"lobby": {
					PublicData: &map[string]interface{}{"ready": float64(2)},
				},
			},
			PublicData:  map[string]interface{}{"round": float64(2)},
			PrivateData: map[string]interface{}{"deck": []interface{}{float64(1), float64(2)}},
			PlayerData:  map[string]map[string]interface{}{"bob": {"warned": true}},
		},
		PublicData:  map[string]interface{}{"name": "A Game", "bar": true},
		PrivateData: map[string]interface{}{"secret": "s"},
		PlayerData:  map[string]interface{}{"alice": map[string]interface{}{"notes": "n"}},
	}
}

// emptyCollectionGame is the shape the fully-populated fixture cannot express:
// an empty Players map and an empty Team.PlayerIDs slice at the same time, plus
// an empty SceneOrder. None of the three carries omitempty, so all three reach
// Lua as {} and only the Go type says which is which.
func emptyCollectionGame(t *testing.T) *models.Game {
	t.Helper()

	id, err := bson.ObjectIDFromHex(testGameID)
	if err != nil {
		t.Fatalf("parsing the fixture object id: %v", err)
	}

	return &models.Game{
		ID:        id,
		Version:   3,
		CreatedAt: time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC),
		Code:      "EFGH",
		Teams:     map[string]models.Team{"red": {Name: "Red", PlayerIDs: []string{}}},
		Players:   map[string]models.Player{},
		Stage:     models.Stage{CurrentScene: "lobby", SceneOrder: []string{}},
	}
}

// runScript hands a game to Lua as the global `state`, runs src against it, and
// returns the table the script leaves behind — the same path a real handler
// takes through gameToLua and back into applyLua.
func runScript(t *testing.T, g *models.Game, src string) lua.LValue {
	t.Helper()

	L := lua.NewState()
	t.Cleanup(L.Close)

	state, err := gameToLua(L, g)
	if err != nil {
		t.Fatalf("gameToLua: %v", err)
	}

	L.SetGlobal("state", state)

	if err := L.DoString(src); err != nil {
		t.Fatalf("running %q: %v", src, err)
	}

	return L.GetGlobal("state")
}

// roundTrip runs src against a copy of g and applies the result, returning the
// mutated game and whatever applyLua reported.
func roundTrip(t *testing.T, g *models.Game, src string) (*models.Game, error) {
	t.Helper()

	state := runScript(t, g, src)

	got := *g

	return &got, applyLua(&got, state, defaultBudget())
}

// TestRoundTrip_FullyPopulatedGameSurvivesUnchanged is the fidelity floor. A
// script that touches nothing must write nothing: events.Diff compares the
// before and after documents with reflect.DeepEqual, so any field that does not
// survive the trip would be published to every player as a spurious change —
// and a lost Version would additionally disarm the CAS fence.
func TestRoundTrip_FullyPopulatedGameSurvivesUnchanged(t *testing.T) {
	want := fullGame(t)

	got, err := roundTrip(t, want, "")
	if err != nil {
		t.Fatalf("applyLua: %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("round trip changed the game\n got: %#v\nwant: %#v", got, want)
	}

	// Called out individually because these four are the ones a JSON round trip
	// cannot carry on its own: Version and DeletedAt are json:"-", ID is a BSON
	// ObjectID, and CreatedAt is restored alongside them.
	if got.Version != want.Version {
		t.Errorf("version = %d, want %d", got.Version, want.Version)
	}

	if got.ID != want.ID {
		t.Errorf("id = %s, want %s", got.ID.Hex(), want.ID.Hex())
	}

	if !got.CreatedAt.Equal(want.CreatedAt) {
		t.Errorf("createdAt = %s, want %s", got.CreatedAt, want.CreatedAt)
	}

	if !got.DeletedAt.Equal(want.DeletedAt) {
		t.Errorf("deletedAt = %s, want %s", got.DeletedAt, want.DeletedAt)
	}
}

// TestRoundTrip_EmptyCollectionsKeepTheirShape covers the case the
// fully-populated fixture structurally cannot: a game whose Players map and
// whose Team.PlayerIDs slice are both empty at once. Lua renders both as {},
// so without the reflect-guided coercion pass one of the two must fail to
// decode, whichever global rule the marshaller picks.
func TestRoundTrip_EmptyCollectionsKeepTheirShape(t *testing.T) {
	want := emptyCollectionGame(t)

	got, err := roundTrip(t, want, "")
	if err != nil {
		t.Fatalf("applyLua: %v", err)
	}

	if got.Players == nil || len(got.Players) != 0 {
		t.Errorf("players = %#v, want an empty non-nil map", got.Players)
	}

	if ids := got.Teams["red"].PlayerIDs; ids == nil || len(ids) != 0 {
		t.Errorf("teams.red.playerIds = %#v, want an empty non-nil slice", ids)
	}

	if order := got.Stage.SceneOrder; order == nil || len(order) != 0 {
		t.Errorf("stage.sceneOrder = %#v, want an empty non-nil slice", order)
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("round trip changed the game\n got: %#v\nwant: %#v", got, want)
	}
}

// TestRoundTrip_ClearedCollectionsMarshalToTheRightJSONShape checks the wire
// shape rather than the Go type, because that shape is what the client parses
// and what events.Diff publishes. An empty sceneOrder must be [] and an empty
// players must be {}; getting either backwards breaks the client's parser.
func TestRoundTrip_ClearedCollectionsMarshalToTheRightJSONShape(t *testing.T) {
	tests := []struct {
		name string
		game func(*testing.T) *models.Game
		src  string
		want string
	}{
		{
			name: "clearing sceneOrder yields an empty array",
			game: fullGame,
			src:  "state.stage.sceneOrder = {}",
			want: `"sceneOrder":[]`,
		},
		{
			// The game starts with no players, so emptying the table is not a
			// membership change and the invariant check lets it through.
			name: "clearing players yields an empty object",
			game: emptyCollectionGame,
			src:  "state.players = {}",
			want: `"players":{}`,
		},
		{
			name: "clearing a team's playerIds yields an empty array",
			game: emptyCollectionGame,
			src:  "state.teams.red.playerIds = {}",
			want: `"playerIds":[]`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := roundTrip(t, test.game(t), test.src)
			if err != nil {
				t.Fatalf("applyLua: %v", err)
			}

			raw, err := json.Marshal(got)
			if err != nil {
				t.Fatalf("marshalling the result: %v", err)
			}

			if !strings.Contains(string(raw), test.want) {
				t.Errorf("marshalled game does not contain %s\ngot: %s", test.want, raw)
			}
		})
	}
}

// TestApplyLua_DeletingAKeyRemovesIt pins down the reason applyLua decodes into
// a zero models.Game. encoding/json merges into an existing map, so decoding
// over the live game would leave every key the script removed in place and no
// deletion would ever reach the database.
func TestApplyLua_DeletingAKeyRemovesIt(t *testing.T) {
	got, err := roundTrip(t, fullGame(t), `
		state.data.bar = nil
		state.stage.scenes.board.privateData.solution = nil
		state.stage.scenes.lobby = nil
	`)
	if err != nil {
		t.Fatalf("applyLua: %v", err)
	}

	if _, ok := got.PublicData["bar"]; ok {
		t.Errorf("data.bar survived deletion: %#v", got.PublicData)
	}

	if got.PublicData["name"] != "A Game" {
		t.Errorf("data.name = %#v, want it left alone", got.PublicData["name"])
	}

	if private := got.Stage.Scenes["board"].PrivateData; private == nil {
		t.Error("stage.scenes.board.privateData disappeared entirely")
	} else if _, ok := (*private)["solution"]; ok {
		t.Errorf("scene privateData.solution survived deletion: %#v", *private)
	}

	if _, ok := got.Stage.Scenes["lobby"]; ok {
		t.Errorf("stage.scenes.lobby survived deletion: %#v", got.Stage.Scenes)
	}
}

// TestApplyLua_ServerOwnedFieldsAreNotForgeable guards the CAS fence. A script
// that could set Version would defeat mutation.Run's optimistic-concurrency
// check and silently overwrite a concurrent write; one that could set ID would
// point the save at another game's document.
func TestApplyLua_ServerOwnedFieldsAreNotForgeable(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{"version", "state.version = 999"},
		{"valid-looking id", `state.id = "aaaaaaaaaaaaaaaaaaaaaaaa"`},
		{"malformed id", `state.id = "not-an-object-id"`},
		{"id removed", "state.id = nil"},
		{"createdAt", `state.createdAt = "1999-01-01T00:00:00Z"`},
		{"deletedAt", `state.deletedAt = "1999-01-01T00:00:00Z"`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			want := fullGame(t)

			got, err := roundTrip(t, want, test.src)
			if err != nil {
				t.Fatalf("applyLua: %v", err)
			}

			if got.Version != want.Version {
				t.Errorf("version = %d, want %d", got.Version, want.Version)
			}

			if got.ID != want.ID {
				t.Errorf("id = %s, want %s", got.ID.Hex(), want.ID.Hex())
			}

			if !got.CreatedAt.Equal(want.CreatedAt) {
				t.Errorf("createdAt = %s, want %s", got.CreatedAt, want.CreatedAt)
			}

			if !got.DeletedAt.Equal(want.DeletedAt) {
				t.Errorf("deletedAt = %s, want %s", got.DeletedAt, want.DeletedAt)
			}
		})
	}
}

// TestApplyLua_RejectsStructuralChanges is the membership and authority fence.
// models.Session stores GameID, TeamID and UserID independently of the game
// document, so a script rewriting membership desynchronises two documents that
// nothing reconciles afterwards — and a script flipping Host would hand itself
// every host-only built-in action.
func TestApplyLua_RejectsStructuralChanges(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{
			name: "adding a player",
			src:  `state.players.carol = { name = "Carol", score = 0, connected = true, host = false, controller = false }`,
		},
		{"removing a player", "state.players.bob = nil"},
		{
			name: "renaming a player",
			src:  "state.players.robert = state.players.bob; state.players.bob = nil",
		},
		{"promoting itself to host", "state.players.bob.host = true"},
		{"demoting the host", "state.players.alice.host = false"},
		{"flipping a connected flag", "state.players.bob.connected = true"},
		{"changing the game code", `state.code = "HACK"`},
		{"changing the game privacy", "state.private = false"},
		{"adding a player to a team", `table.insert(state.teams.red.playerIds, "bob")`},
		{"removing a player from a team", "state.teams.red.playerIds = {}"},
		{"moving a player between teams", `state.teams.red.playerIds = {"bob"}; state.teams.blue.playerIds = {"alice"}`},
		{"dropping a team that still holds players", "state.teams.red = nil"},
		{
			name: "adding a team that already holds players",
			src:  `state.teams.green = { name = "Green", playerIds = {"alice"} }`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			want := fullGame(t)

			got, err := roundTrip(t, want, test.src)
			if err == nil {
				t.Fatalf("applyLua accepted %q, want it rejected", test.src)
			}

			// Rejection means no write at all, not a partial one: the caller
			// turns the error into a script error and Store.Mutate must see the
			// game exactly as it loaded it.
			if !reflect.DeepEqual(got, want) {
				t.Errorf("rejected apply still modified the game\n got: %#v\nwant: %#v", got, want)
			}
		})
	}
}

// TestApplyLua_AcceptsContentChanges is the other half of the fence. The
// invariant check exists to keep membership in Go, not to make the state
// read-only: a game's own content — scores, every *Data map and the whole Stage
// — is exactly what a script is for.
func TestApplyLua_AcceptsContentChanges(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		check func(*testing.T, *models.Game)
	}{
		{
			name: "setting a player score",
			src:  "state.players.alice.score = 42",
			check: func(t *testing.T, g *models.Game) {
				if got := g.Players["alice"].Score; got != 42 {
					t.Errorf("alice score = %d, want 42", got)
				}
			},
		},
		{
			name: "writing scene data",
			src:  `state.stage.scenes.board.data.winner = "x"`,
			check: func(t *testing.T, g *models.Game) {
				data := g.Stage.Scenes["board"].PublicData
				if data == nil || (*data)["winner"] != "x" {
					t.Errorf("scene data = %#v, want winner x", data)
				}
			},
		},
		{
			name: "advancing the current scene",
			src:  `state.stage.currentScene = "results"`,
			check: func(t *testing.T, g *models.Game) {
				if g.Stage.CurrentScene != "results" {
					t.Errorf("currentScene = %q, want results", g.Stage.CurrentScene)
				}
			},
		},
		{
			name: "reordering the scenes",
			src:  `state.stage.sceneOrder = {"board", "lobby"}`,
			check: func(t *testing.T, g *models.Game) {
				if want := []string{"board", "lobby"}; !reflect.DeepEqual(g.Stage.SceneOrder, want) {
					t.Errorf("sceneOrder = %#v, want %#v", g.Stage.SceneOrder, want)
				}
			},
		},
		{
			name: "writing game public data",
			src:  "state.data.round = 3",
			check: func(t *testing.T, g *models.Game) {
				if got := g.PublicData["round"]; got != float64(3) {
					t.Errorf("data.round = %#v, want 3", got)
				}
			},
		},
		{
			name: "writing game private data",
			src:  `state.privateData.secret = "t"`,
			check: func(t *testing.T, g *models.Game) {
				if got := g.PrivateData["secret"]; got != "t" {
					t.Errorf("privateData.secret = %#v, want t", got)
				}
			},
		},
		{
			name: "writing per-player data",
			src:  `state.players.bob.data.colour = "green"`,
			check: func(t *testing.T, g *models.Game) {
				data := g.Players["bob"].PublicData
				if data == nil || (*data)["colour"] != "green" {
					t.Errorf("bob data = %#v, want colour green", data)
				}
			},
		},
		{
			name: "writing team data",
			src:  "state.teams.red.data.turn = false",
			check: func(t *testing.T, g *models.Game) {
				if got := g.Teams["red"].PublicData["turn"]; got != false {
					t.Errorf("teams.red.data.turn = %#v, want false", got)
				}
			},
		},
		{
			name: "adding an empty team",
			src:  `state.teams.green = { name = "Green", playerIds = {} }`,
			check: func(t *testing.T, g *models.Game) {
				team, ok := g.Teams["green"]
				if !ok {
					t.Fatalf("teams.green is missing: %#v", g.Teams)
				}

				if team.PlayerIDs == nil || len(team.PlayerIDs) != 0 {
					t.Errorf("teams.green.playerIds = %#v, want an empty non-nil slice", team.PlayerIDs)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := roundTrip(t, fullGame(t), test.src)
			if err != nil {
				t.Fatalf("applyLua rejected %q: %v", test.src, err)
			}

			test.check(t, got)
		})
	}
}

// TestApplyLua_RejectsNonTableState guards the entry condition: a script that
// returns a scalar must produce a named error rather than a confusing JSON
// decode failure further down.
func TestApplyLua_RejectsNonTableState(t *testing.T) {
	g := fullGame(t)

	if err := applyLua(g, lua.LString("nope"), defaultBudget()); err == nil {
		t.Fatal("applyLua accepted a string as game state, want an error")
	}
}

// TestCoerceEmpty_OnlyActsWhereTheTargetTypeIsConcrete is the unit-level
// statement of the coercion rule. Inside a *Data map the target is interface{},
// which accepts either shape, so coercing there would invent a change the
// script never made.
func TestCoerceEmpty_OnlyActsWhereTheTargetTypeIsConcrete(t *testing.T) {
	gameType := reflect.TypeOf(models.Game{})

	tests := []struct {
		name string
		doc  map[string]interface{}
		want map[string]interface{}
	}{
		{
			name: "empty array becomes an empty map for a map field",
			doc:  map[string]interface{}{"players": []interface{}{}},
			want: map[string]interface{}{"players": map[string]interface{}{}},
		},
		{
			name: "empty map becomes an empty array for a slice field",
			doc:  map[string]interface{}{"stage": map[string]interface{}{"sceneOrder": map[string]interface{}{}}},
			want: map[string]interface{}{"stage": map[string]interface{}{"sceneOrder": []interface{}{}}},
		},
		{
			name: "coercion reaches through a map of structs",
			doc: map[string]interface{}{
				"teams": map[string]interface{}{"red": map[string]interface{}{"playerIds": map[string]interface{}{}}},
			},
			want: map[string]interface{}{
				"teams": map[string]interface{}{"red": map[string]interface{}{"playerIds": []interface{}{}}},
			},
		},
		{
			// Scene.PublicData is a *map, so the walk has to follow the pointer
			// to find the target kind at all.
			name: "coercion reaches through a pointer field",
			doc: map[string]interface{}{
				"stage": map[string]interface{}{
					"scenes": map[string]interface{}{"board": map[string]interface{}{"data": []interface{}{}}},
				},
			},
			want: map[string]interface{}{
				"stage": map[string]interface{}{
					"scenes": map[string]interface{}{"board": map[string]interface{}{"data": map[string]interface{}{}}},
				},
			},
		},
		{
			name: "an empty map inside data is left alone",
			doc:  map[string]interface{}{"data": map[string]interface{}{"flags": map[string]interface{}{}}},
			want: map[string]interface{}{"data": map[string]interface{}{"flags": map[string]interface{}{}}},
		},
		{
			name: "an empty array inside data is left alone",
			doc:  map[string]interface{}{"data": map[string]interface{}{"log": []interface{}{}}},
			want: map[string]interface{}{"data": map[string]interface{}{"log": []interface{}{}}},
		},
		{
			name: "a non-empty collection is never reshaped",
			doc:  map[string]interface{}{"stage": map[string]interface{}{"sceneOrder": []interface{}{"lobby"}}},
			want: map[string]interface{}{"stage": map[string]interface{}{"sceneOrder": []interface{}{"lobby"}}},
		},
		{
			name: "an unknown key has no target type and is untouched",
			doc:  map[string]interface{}{"version": float64(999), "nonsense": []interface{}{}},
			want: map[string]interface{}{"version": float64(999), "nonsense": []interface{}{}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := coerceEmpty(test.doc, gameType)

			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("coerceEmpty() = %#v, want %#v", got, test.want)
			}
		})
	}
}
