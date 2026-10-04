package leave

import (
	"testing"

	"github.com/robbiebyrd/indri/internal/handlers/actions/join"
	"github.com/robbiebyrd/indri/internal/handlers/handlertest"
	"github.com/robbiebyrd/indri/internal/models"
)

// seated joins name to red in game JOINME and returns their session as the
// store now holds it.
func seated(t *testing.T, f *handlertest.Fixture, name string) *models.Session {
	t.Helper()

	session := f.Player(t, name)

	if _, err := f.Do(join.New(f.I), session, map[string]interface{}{"code": "JOINME", "teamId": "red"}); err != nil {
		t.Fatalf("seating %s: %v", name, err)
	}

	return f.Session(t, session.ID)
}

// Leaving frees the seat and takes the session out of the game, so it stops
// receiving that game's broadcasts.
func TestLeave_FreesTheSeatAndTakesTheSessionOutOfTheGame(t *testing.T) {
	f := handlertest.New(t, handlertest.TwoTeams())
	g := f.Game(t, "JOINME")
	ada := seated(t, f, "ada")
	slot := *ada.SlotID

	if _, err := f.Do(New(f.I), ada, nil); err != nil {
		t.Fatalf("leave: %v", err)
	}

	if holder := f.GameByID(t, g.ID).Players[slot].UserID; holder != "" {
		t.Fatalf("slot %s is still held by %q after leaving", slot, holder)
	}

	if session := f.Session(t, ada.ID); session.GameID != nil || session.SlotID != nil || session.TeamID != nil {
		t.Fatalf("the session still names game %v, team %v, slot %v", session.GameID, session.TeamID, session.SlotID)
	}
}

// A session whose seat is already gone — and taken by someone else — leaves
// cleanly, and the new holder keeps the seat.
func TestLeave_FromASeatSomeoneElseNowHolds(t *testing.T) {
	f := handlertest.New(t, handlertest.TwoTeams())
	g := f.Game(t, "JOINME")
	ada := seated(t, f, "ada")
	slot := *ada.SlotID

	if err := f.I.GameService.RemovePlayer(g.ID, slot, *ada.UserID); err != nil {
		t.Fatalf("freeing ada's seat behind her back: %v", err)
	}

	bea := seated(t, f, "bea")
	if *bea.SlotID != slot {
		t.Fatalf("bea got %s, want ada's old slot %s", *bea.SlotID, slot)
	}

	if _, err := f.Do(New(f.I), ada, nil); err != nil {
		t.Fatalf("leave from a lost seat: %v", err)
	}

	if holder := f.GameByID(t, g.ID).Players[slot].UserID; holder != *bea.UserID {
		t.Fatalf("slot %s is held by %q, want still bea", slot, holder)
	}

	if session := f.Session(t, ada.ID); session.GameID != nil {
		t.Fatalf("ada's session still names game %s", *session.GameID)
	}
}

func TestLeave_RefusesACallerInNoGame(t *testing.T) {
	f := handlertest.New(t, handlertest.TwoTeams())

	if _, err := f.Do(New(f.I), f.Player(t, "ada"), nil); err == nil {
		t.Fatal("leaving with no game succeeded")
	}

	if _, err := f.Do(New(f.I), nil, nil); err == nil {
		t.Fatal("an unauthenticated leave succeeded")
	}
}
