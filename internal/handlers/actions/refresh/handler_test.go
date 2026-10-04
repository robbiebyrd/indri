package refresh

import (
	"strings"
	"testing"

	"github.com/robbiebyrd/indri/internal/handlers/handlertest"
)

func TestRefresh_SendsTheKeyframeToAPlayerInTheirSeat(t *testing.T) {
	f := handlertest.New(t, handlertest.TwoTeams())
	g := f.Game(t, "GAME")
	ada := f.Seat(t, "ada", g.ID, "red")

	result, err := f.Do(New(f.I), ada, nil)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}

	if len(result.Responses) != 1 || !strings.Contains(string(result.Responses[0]), g.ID) {
		t.Fatalf("refresh answered %q, want the game's keyframe", result.Responses)
	}
}

// A keyframe is the whole game; a session that outlived its seat is no longer
// a player of it and must not be sent one.
func TestRefresh_RefusesASessionThatLostItsSeat(t *testing.T) {
	f := handlertest.New(t, handlertest.TwoTeams())
	g := f.Game(t, "GAME")
	ada := f.Seat(t, "ada", g.ID, "red")
	f.LoseSeat(t, ada)

	result, err := f.Do(New(f.I), ada, nil)
	if err == nil {
		t.Fatal("refresh from a lost seat succeeded")
	}

	if len(result.Responses) != 1 || strings.Contains(string(result.Responses[0]), g.ID) {
		t.Fatalf("refresh from a lost seat answered %q, want an error and no keyframe", result.Responses)
	}
}
