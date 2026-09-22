package luahandler

import (
	"testing"

	lua "github.com/yuin/gopher-lua"

	"github.com/robbiebyrd/indri/internal/models"
)

func makeTestGame() *models.Game {
	board := []interface{}{
		[]interface{}{"X", "", ""},
		[]interface{}{"", "O", ""},
		[]interface{}{"", "", ""},
	}
	sceneData := map[string]interface{}{"board": board}
	scene := models.Scene{PublicData: &sceneData}
	return &models.Game{
		Teams: map[string]models.Team{
			"P1": {PublicData: map[string]interface{}{"marker": "X", "turn": true}},
			"P2": {PublicData: map[string]interface{}{"marker": "O", "turn": false}},
		},
		Stage: models.Stage{
			CurrentScene: "board",
			Scenes:       map[string]models.Scene{"board": scene},
		},
	}
}

func TestGameToLua_TeamDataPresent(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	g := makeTestGame()
	tbl, err := gameToLua(L, g)
	if err != nil {
		t.Fatalf("gameToLua: %v", err)
	}

	teams := tbl.RawGetString("teams").(*lua.LTable)
	p1 := teams.RawGetString("P1").(*lua.LTable)
	data := p1.RawGetString("data").(*lua.LTable)
	marker := data.RawGetString("marker")
	if marker.String() != "X" {
		t.Errorf("expected marker X, got %v", marker)
	}
	turn := data.RawGetString("turn")
	if turn != lua.LTrue {
		t.Errorf("expected turn=true, got %v", turn)
	}
}

func TestGameToLua_SceneDataPresent(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	g := makeTestGame()
	tbl, err := gameToLua(L, g)
	if err != nil {
		t.Fatalf("gameToLua: %v", err)
	}

	scene := tbl.RawGetString("scene").(*lua.LTable)
	data := scene.RawGetString("data").(*lua.LTable)
	board := data.RawGetString("board").(*lua.LTable)
	row1 := board.RawGetInt(1).(*lua.LTable)
	cell := row1.RawGetInt(1)
	if cell.String() != "X" {
		t.Errorf("expected board[1][1]=X, got %v", cell)
	}
}

func TestGameToLua_CurrentScene(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	g := makeTestGame()
	tbl, err := gameToLua(L, g)
	if err != nil {
		t.Fatalf("gameToLua: %v", err)
	}

	stage := tbl.RawGetString("stage").(*lua.LTable)
	cs := stage.RawGetString("currentScene")
	if cs.String() != "board" {
		t.Errorf("expected currentScene=board, got %v", cs)
	}
}

func TestApplyLuaToGame_UpdatesTurn(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	g := makeTestGame()
	tbl, err := gameToLua(L, g)
	if err != nil {
		t.Fatalf("gameToLua: %v", err)
	}

	// Flip P1's turn to false
	teams := tbl.RawGetString("teams").(*lua.LTable)
	p1 := teams.RawGetString("P1").(*lua.LTable)
	p1.RawGetString("data").(*lua.LTable).RawSetString("turn", lua.LFalse)

	if err := applyLuaToGame(L, tbl, g); err != nil {
		t.Fatalf("applyLuaToGame: %v", err)
	}

	if g.Teams["P1"].PublicData["turn"] != false {
		t.Errorf("expected P1.turn=false after apply, got %v", g.Teams["P1"].PublicData["turn"])
	}
}

func TestApplyLuaToGame_UpdatesBoard(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	g := makeTestGame()
	tbl, err := gameToLua(L, g)
	if err != nil {
		t.Fatalf("gameToLua: %v", err)
	}

	// Set board[1][2] = "X" (1-indexed)
	scene := tbl.RawGetString("scene").(*lua.LTable)
	board := scene.RawGetString("data").(*lua.LTable).RawGetString("board").(*lua.LTable)
	board.RawGetInt(1).(*lua.LTable).RawSetInt(2, lua.LString("X"))

	if err := applyLuaToGame(L, tbl, g); err != nil {
		t.Fatalf("applyLuaToGame: %v", err)
	}

	sceneData := *g.Stage.Scenes["board"].PublicData
	boardBack := sceneData["board"].([]interface{})
	row0 := boardBack[0].([]interface{})
	if row0[1] != "X" {
		t.Errorf("expected board[0][1]=X, got %v", row0[1])
	}
}

func TestApplyLuaToGame_NilRemovesKey(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	g := makeTestGame()
	// Manually add winningTeam
	sd := *g.Stage.Scenes["board"].PublicData
	sd["winningTeam"] = "P1"
	scene := g.Stage.Scenes["board"]
	scene.PublicData = &sd
	g.Stage.Scenes["board"] = scene

	tbl, err := gameToLua(L, g)
	if err != nil {
		t.Fatalf("gameToLua: %v", err)
	}

	// Remove winningTeam from Lua (nil = delete)
	tbl.RawGetString("scene").(*lua.LTable).
		RawGetString("data").(*lua.LTable).
		RawSetString("winningTeam", lua.LNil)

	if err := applyLuaToGame(L, tbl, g); err != nil {
		t.Fatalf("applyLuaToGame: %v", err)
	}

	finalData := *g.Stage.Scenes["board"].PublicData
	if _, ok := finalData["winningTeam"]; ok {
		t.Error("expected winningTeam to be removed, but it still exists")
	}
}
