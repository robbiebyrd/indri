package luahandler

import (
	"encoding/json"
	"fmt"

	lua "github.com/yuin/gopher-lua"

	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/services/events"
)

// jsonToLua converts a plain Go value (as produced by json.Unmarshal) into a
// Lua value.  Arrays become 1-indexed tables; nil becomes LNil.
func jsonToLua(L *lua.LState, v interface{}) lua.LValue {
	switch val := v.(type) {
	case map[string]interface{}:
		t := L.NewTable()
		for k, sub := range val {
			t.RawSetString(k, jsonToLua(L, sub))
		}
		return t
	case []interface{}:
		t := L.NewTable()
		for i, sub := range val {
			t.RawSetInt(i+1, jsonToLua(L, sub))
		}
		return t
	case string:
		return lua.LString(val)
	case float64:
		return lua.LNumber(val)
	case bool:
		if val {
			return lua.LTrue
		}
		return lua.LFalse
	default:
		return lua.LNil
	}
}

// luaToJson converts a Lua value back to a plain Go value.  Array tables
// (positive MaxN) become []interface{}; object tables become
// map[string]interface{}; nil keys are omitted naturally via ForEach.
func luaToJson(L *lua.LState, v lua.LValue) interface{} {
	switch val := v.(type) {
	case *lua.LTable:
		if val.MaxN() > 0 {
			n := val.MaxN()
			arr := make([]interface{}, n)
			for i := 1; i <= n; i++ {
				arr[i-1] = luaToJson(L, val.RawGetInt(i))
			}
			return arr
		}
		m := make(map[string]interface{})
		val.ForEach(func(k lua.LValue, elem lua.LValue) {
			m[k.String()] = luaToJson(L, elem)
		})
		return m
	case lua.LString:
		return string(val)
	case lua.LNumber:
		return float64(val)
	case lua.LBool:
		return bool(val)
	default:
		return nil
	}
}

// gameToLua builds the Lua "game" table that handler scripts read and mutate.
// It exposes team public data, the current scene (public + private data), and
// the stage header.
func gameToLua(L *lua.LState, g *models.Game) (*lua.LTable, error) {
	gMap, err := events.ToMap(g)
	if err != nil {
		return nil, fmt.Errorf("normalizing game: %w", err)
	}

	gameTable := L.NewTable()

	// teams
	teamsTable := L.NewTable()
	if tv, ok := gMap["teams"]; ok && tv != nil {
		for teamId, teamVal := range tv.(map[string]interface{}) {
			teamMap := teamVal.(map[string]interface{})
			t := L.NewTable()
			if d, ok := teamMap["data"]; ok {
				t.RawSetString("data", jsonToLua(L, d))
			} else {
				t.RawSetString("data", L.NewTable())
			}
			teamsTable.RawSetString(teamId, t)
		}
	}
	gameTable.RawSetString("teams", teamsTable)

	// current scene
	sceneTable := L.NewTable()
	var stageMap map[string]interface{}
	if sv, ok := gMap["stage"]; ok && sv != nil {
		stageMap = sv.(map[string]interface{})
		currentScene, _ := stageMap["currentScene"].(string)
		if scenesVal, ok := stageMap["scenes"]; ok && scenesVal != nil {
			scenesMap := scenesVal.(map[string]interface{})
			if sv2, ok := scenesMap[currentScene]; ok && sv2 != nil {
				sm := sv2.(map[string]interface{})
				if d, ok := sm["data"]; ok {
					sceneTable.RawSetString("data", jsonToLua(L, d))
				}
				if pd, ok := sm["privateData"]; ok {
					sceneTable.RawSetString("privateData", jsonToLua(L, pd))
				}
			}
		}
	}
	gameTable.RawSetString("scene", sceneTable)

	// stage header
	stageTable := L.NewTable()
	if stageMap != nil {
		if cs, ok := stageMap["currentScene"].(string); ok {
			stageTable.RawSetString("currentScene", lua.LString(cs))
		}
	}
	gameTable.RawSetString("stage", stageTable)

	return gameTable, nil
}

// applyLuaToGame writes the Lua game table back into the Go game object,
// updating team public data and the current scene's public and private data.
func applyLuaToGame(L *lua.LState, gameTable *lua.LTable, g *models.Game) error {
	if teamsTable, ok := gameTable.RawGetString("teams").(*lua.LTable); ok {
		teamsTable.ForEach(func(k lua.LValue, v lua.LValue) {
			teamId := k.String()
			teamTable, ok := v.(*lua.LTable)
			if !ok {
				return
			}
			team, exists := g.Teams[teamId]
			if !exists {
				return
			}
			if dataVal := teamTable.RawGetString("data"); dataVal != lua.LNil {
				if dataTable, ok := dataVal.(*lua.LTable); ok {
					team.PublicData = luaToJson(L, dataTable).(map[string]interface{})
				}
			}
			g.Teams[teamId] = team
		})
	}

	sceneTable, ok := gameTable.RawGetString("scene").(*lua.LTable)
	if !ok {
		return nil
	}

	currentScene := g.Stage.CurrentScene
	scene := g.Stage.Scenes[currentScene]

	if dataVal := sceneTable.RawGetString("data"); dataVal != lua.LNil {
		if dataTable, ok := dataVal.(*lua.LTable); ok {
			m := luaToJson(L, dataTable).(map[string]interface{})
			scene.PublicData = &m
		}
	}

	if pdVal := sceneTable.RawGetString("privateData"); pdVal != lua.LNil {
		if pdTable, ok := pdVal.(*lua.LTable); ok {
			m := luaToJson(L, pdTable).(map[string]interface{})
			scene.PrivateData = &m
		}
	}

	g.Stage.Scenes[currentScene] = scene
	return nil
}

// buildInitialTable constructs the Lua "initial" table with the script's
// default team state so handler scripts can reference it for resets.
func buildInitialTable(L *lua.LState, script *models.Script) (*lua.LTable, error) {
	raw, err := json.Marshal(script)
	if err != nil {
		return nil, fmt.Errorf("marshaling script: %w", err)
	}
	var scriptMap map[string]interface{}
	if err := json.Unmarshal(raw, &scriptMap); err != nil {
		return nil, fmt.Errorf("unmarshaling script: %w", err)
	}

	initial := L.NewTable()
	teamsTable := L.NewTable()
	if tv, ok := scriptMap["teams"]; ok && tv != nil {
		for teamId, teamVal := range tv.(map[string]interface{}) {
			teamMap := teamVal.(map[string]interface{})
			t := L.NewTable()
			if d, ok := teamMap["data"]; ok {
				t.RawSetString("data", jsonToLua(L, d))
			} else {
				t.RawSetString("data", L.NewTable())
			}
			teamsTable.RawSetString(teamId, t)
		}
	}
	initial.RawSetString("teams", teamsTable)

	return initial, nil
}
