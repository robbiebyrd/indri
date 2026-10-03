package join

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/robbiebyrd/indri/internal/handlers/handlertest"
	"github.com/robbiebyrd/indri/internal/models"
)

func str(s *string) string {
	if s == nil {
		return "<nil>"
	}

	return *s
}

func TestJoin_SeatsTheCallerAndRecordsTheSeatOnTheirSession(t *testing.T) {
	f := handlertest.New(t, handlertest.TwoTeams())
	g := f.Game(t, "JOINME")
	ada := f.Player(t, "ada")

	result, err := f.Do(New(f.I), ada, map[string]interface{}{"code": "JOINME", "teamId": "red"})
	if err != nil {
		t.Fatalf("join: %v", err)
	}

	session := f.Session(t, ada.ID)
	if str(session.GameID) != g.ID || str(session.TeamID) != "red" || session.SlotID == nil {
		t.Fatalf("session after join = game %s, team %s, slot %s; want %s, red and a slot",
			str(session.GameID), str(session.TeamID), str(session.SlotID), g.ID)
	}

	stored := f.GameByID(t, g.ID)
	if holder := stored.Players[*session.SlotID].UserID; holder != *ada.UserID {
		t.Fatalf("slot %s is held by %q, want ada (%s)", *session.SlotID, holder, *ada.UserID)
	}

	if len(result.Responses) != 1 {
		t.Fatalf("join answered with %d responses, want the keyframe", len(result.Responses))
	}

	var keyframe models.Game
	if err := json.Unmarshal(result.Responses[0], &keyframe); err != nil {
		t.Fatalf("decoding the keyframe: %v", err)
	}

	if keyframe.Players[*session.SlotID].UserID != *ada.UserID {
		t.Errorf("the keyframe was taken before ada was seated")
	}
}

func TestJoin_RejoiningKeepsTheSameSlot(t *testing.T) {
	f := handlertest.New(t, handlertest.TwoTeams())
	f.Game(t, "JOINME")
	ada := f.Player(t, "ada")

	payload := map[string]interface{}{"code": "JOINME", "teamId": "red"}

	if _, err := f.Do(New(f.I), ada, payload); err != nil {
		t.Fatalf("first join: %v", err)
	}

	first := f.Session(t, ada.ID).SlotID

	if _, err := f.Do(New(f.I), f.Session(t, ada.ID), payload); err != nil {
		t.Fatalf("second join: %v", err)
	}

	if again := f.Session(t, ada.ID).SlotID; str(again) != str(first) {
		t.Fatalf("rejoining moved ada from slot %s to %s", str(first), str(again))
	}
}

// A join that names no team is refused before anything is written: there is no
// slot to give a player who picked no team.
func TestJoin_RefusesAJoinWithNoTeam(t *testing.T) {
	f := handlertest.New(t, handlertest.TwoTeams())
	f.Game(t, "JOINME")
	ada := f.Player(t, "ada")

	if _, err := f.Do(New(f.I), ada, map[string]interface{}{"code": "JOINME"}); err == nil {
		t.Fatal("a join with no team succeeded")
	}

	if session := f.Session(t, ada.ID); session.GameID != nil {
		t.Fatalf("a refused join still put the session in game %s", *session.GameID)
	}
}

// A caller who could not be seated must not have a session claiming a game.
func TestJoin_AFullTeamLeavesTheSessionOutOfTheGame(t *testing.T) {
	f := handlertest.New(t, handlertest.TwoTeams())
	f.Game(t, "JOINME")

	for _, name := range []string{"ada", "bea"} {
		if _, err := f.Do(New(f.I), f.Player(t, name), map[string]interface{}{"code": "JOINME", "teamId": "red"}); err != nil {
			t.Fatalf("seating %s: %v", name, err)
		}
	}

	cyd := f.Player(t, "cyd")

	if _, err := f.Do(New(f.I), cyd, map[string]interface{}{"code": "JOINME", "teamId": "red"}); err == nil {
		t.Fatal("joining a full team succeeded")
	}

	if session := f.Session(t, cyd.ID); session.GameID != nil || session.SlotID != nil {
		t.Fatalf("a refused join left the session naming game %s, slot %s", str(session.GameID), str(session.SlotID))
	}
}

func TestJoin_AnUnauthenticatedCallerIsSentToLogin(t *testing.T) {
	f := handlertest.New(t, handlertest.TwoTeams())
	f.Game(t, "JOINME")

	result, err := f.Do(New(f.I), nil, map[string]interface{}{"code": "JOINME", "teamId": "red"})
	if err == nil {
		t.Fatal("an unauthenticated join succeeded")
	}

	if len(result.Responses) != 1 || !strings.Contains(string(result.Responses[0]), `"currentScene": "login"`) {
		t.Fatalf("an unauthenticated join answered %q, want the login scene", result.Responses)
	}
}
