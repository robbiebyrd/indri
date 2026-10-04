package utils

import (
	"fmt"

	"github.com/robbiebyrd/indri/internal/models"
)

// RequireHost checks that the caller hosts g. The caller is their own session —
// resolved by the transport from their connection, never from anything in the
// message — and is found in the game by the slot that session names.
//
// Naming the slot is not enough: a session can outlive its seat, so the slot
// must still be held by the session's own user before its host flag counts.
func RequireHost(session *models.Session, g *models.Game) error {
	if session == nil {
		return fmt.Errorf("must be logged in")
	}

	if session.UserID == nil || *session.UserID == "" {
		return fmt.Errorf("calling session has no user id")
	}

	if session.GameID == nil || *session.GameID != g.ID {
		return fmt.Errorf("caller %v is not in game %v", *session.UserID, g.Code)
	}

	if session.SlotID == nil || *session.SlotID == "" {
		return fmt.Errorf("caller %v has no slot in game %v", *session.UserID, g.Code)
	}

	if !HoldsSeat(g, session) {
		return fmt.Errorf("caller %v does not hold slot %v of game %v", *session.UserID, *session.SlotID, g.Code)
	}

	if !g.Players[*session.SlotID].Host {
		return fmt.Errorf("caller %v is not the host of game %v", *session.UserID, g.Code)
	}

	return nil
}
