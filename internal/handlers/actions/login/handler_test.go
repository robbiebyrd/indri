package login

import (
	"testing"

	"github.com/robbiebyrd/indri/internal/handlers/handlertest"
)

// Logging in hands back the user's one session. One that outlived its seat
// stops naming the game, so the new connection does not receive its broadcasts.
func TestLogin_DropsASeatTheUserNoLongerHolds(t *testing.T) {
	f := handlertest.New(t, handlertest.TwoTeams())
	g := f.Game(t, "GAME")
	ada := f.Seat(t, "ada", g.ID, "red")
	f.LoseSeat(t, ada)

	if _, err := f.Do(New(f.I), nil, f.Credentials(ada)); err != nil {
		t.Fatalf("login: %v", err)
	}

	if session := f.Session(t, ada.ID); session.GameID != nil || session.SlotID != nil {
		t.Fatalf("the session still names game %v, slot %v", session.GameID, session.SlotID)
	}
}

func TestLogin_KeepsASeatTheUserStillHolds(t *testing.T) {
	f := handlertest.New(t, handlertest.TwoTeams())
	g := f.Game(t, "GAME")
	ada := f.Seat(t, "ada", g.ID, "red")

	if _, err := f.Do(New(f.I), nil, f.Credentials(ada)); err != nil {
		t.Fatalf("login: %v", err)
	}

	if session := f.Session(t, ada.ID); session.GameID == nil || *session.GameID != g.ID {
		t.Fatalf("logging in again took a seated user out of their game")
	}
}
