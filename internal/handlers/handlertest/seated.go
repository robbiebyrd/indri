package handlertest

import (
	"testing"

	"github.com/robbiebyrd/indri/internal/models"
)

// Seat puts name in game gameId on teamId through the game and session
// services, the way join does, and returns their session as now stored.
func (f *Fixture) Seat(t *testing.T, name string, gameId string, teamId string) *models.Session {
	t.Helper()

	session := f.Player(t, name)

	slot, err := f.I.GameService.ConnectPlayer(gameId, teamId, *session.UserID, name)
	if err != nil {
		t.Fatalf("seating %s: %v", name, err)
	}

	if err := f.I.SessionService.Update(session.ID, &models.UpdateSession{GameID: gameId, TeamID: teamId, SlotID: slot}); err != nil {
		t.Fatalf("recording %s's seat: %v", name, err)
	}

	return f.Session(t, session.ID)
}

// LoseSeat frees session's seat behind its back, as a kick or a leave from
// another connection would, leaving the session still naming the game.
func (f *Fixture) LoseSeat(t *testing.T, session *models.Session) {
	t.Helper()

	if err := f.I.GameService.RemovePlayer(*session.GameID, *session.SlotID, *session.UserID); err != nil {
		t.Fatalf("freeing the seat: %v", err)
	}
}
