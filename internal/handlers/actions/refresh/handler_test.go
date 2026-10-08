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

// A player who says which layout they hold is sent the keyframe without it;
// one who names none is sent it.
func TestRefresh_LeavesOutTheLayoutTheCallerHolds(t *testing.T) {
	f := handlertest.New(t, handlertest.WithLayout())
	g := f.Game(t, "GAME")
	ada := f.Seat(t, "ada", g.ID, "red")

	full, err := f.Do(New(f.I), ada, nil)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}

	if !handlertest.HasLayout(t, full.Responses[0]) {
		t.Fatal("a refresh naming no layout was not sent the layout")
	}

	hash := handlertest.LayoutHash(t, full.Responses[0])

	lean, err := f.Do(New(f.I), ada, map[string]interface{}{"layoutHash": hash})
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}

	if handlertest.HasLayout(t, lean.Responses[0]) {
		t.Error("a refresh naming the layout it holds was sent it again")
	}
}
