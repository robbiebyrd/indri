package reconnect

import (
	"testing"

	"github.com/robbiebyrd/indri/internal/handlers/handlertest"
)

func TestReconnect_ResumesIntoAHeldSeatWithTheGame(t *testing.T) {
	f := handlertest.New(t, handlertest.TwoTeams())
	g := f.Game(t, "GAME")
	ada := f.Seat(t, "ada", g.ID, "red")

	result, err := f.Do(New(f.I), nil, map[string]interface{}{"sessionId": ada.Token})
	if err != nil {
		t.Fatalf("reconnect: %v", err)
	}

	if len(result.Responses) != 2 {
		t.Fatalf("reconnect answered %d messages, want the login and the game's keyframe", len(result.Responses))
	}
}

// A session that outlived its seat resumes outside the game: no keyframe of a
// game the user is no longer in, and no more of its broadcasts.
func TestReconnect_ResumesALostSeatOutsideTheGame(t *testing.T) {
	f := handlertest.New(t, handlertest.TwoTeams())
	g := f.Game(t, "GAME")
	ada := f.Seat(t, "ada", g.ID, "red")
	f.LoseSeat(t, ada)

	result, err := f.Do(New(f.I), nil, map[string]interface{}{"sessionId": ada.Token})
	if err != nil {
		t.Fatalf("reconnect: %v", err)
	}

	if len(result.Responses) != 1 {
		t.Fatalf("reconnect answered %d messages, want the login alone", len(result.Responses))
	}

	if session := f.Session(t, ada.ID); session.GameID != nil {
		t.Fatalf("the session still names game %s", *session.GameID)
	}
}
