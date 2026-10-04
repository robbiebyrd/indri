package script

import (
	"strings"
	"testing"

	"github.com/robbiebyrd/indri/internal/handlers/handlertest"
	"github.com/robbiebyrd/indri/internal/models"
)

// moveScript answers every move, so a reply proves the script ran.
const moveScript = `indri.on("move", function(req) indri.reply({moved = true}) end)`

func scriptFixture(t *testing.T) *handlertest.Fixture {
	t.Helper()

	f := handlertest.New(t, handlertest.TwoTeams())
	f.I.LuaEngine = luaEngine(t, moveScript)

	return f
}

func moved(t *testing.T, responses [][]byte) bool {
	t.Helper()

	return len(responses) == 1 && strings.Contains(string(responses[0]), `"moved"`)
}

func TestHandle_RunsTheScriptForAPlayerInTheirSeat(t *testing.T) {
	f := scriptFixture(t)
	g := f.Game(t, "GAME")
	ada := f.Seat(t, "ada", g.ID, "red")

	result, err := f.Do(New(f.I, "move"), ada, map[string]interface{}{})
	if err != nil || !moved(t, result.Responses) {
		t.Fatalf("move from a held seat = %q, %v; want the script's reply", result.Responses, err)
	}
}

// A game action is taken for a player of the game. A session that outlived its
// seat is no longer one, and must not get to act in it.
func TestHandle_RefusesASessionThatLostItsSeat(t *testing.T) {
	f := scriptFixture(t)
	g := f.Game(t, "GAME")
	ada := f.Seat(t, "ada", g.ID, "red")
	f.LoseSeat(t, ada)

	result, err := f.Do(New(f.I, "move"), ada, map[string]interface{}{})
	if err == nil {
		t.Fatal("a move from a lost seat succeeded")
	}

	if moved(t, result.Responses) {
		t.Fatal("the script ran for a session that lost its seat")
	}
}

// Who may act outside a game is the script's to decide, so callers in no game —
// and unauthenticated ones — reach it.
func TestHandle_LeavesCallersInNoGameToTheScript(t *testing.T) {
	f := scriptFixture(t)

	for name, session := range map[string]*models.Session{"a lobby session": f.Player(t, "ada"), "no session": nil} {
		t.Run(name, func(t *testing.T) {
			result, err := f.Do(New(f.I, "move"), session, map[string]interface{}{})
			if err != nil || !moved(t, result.Responses) {
				t.Fatalf("move from %s = %q, %v; want the script's reply", name, result.Responses, err)
			}
		})
	}
}
