package session

import (
	"time"

	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/repo/ids"
)

// Session field handling shared by every store, so each one keeps the same
// fields.

// newSession builds a session from c with a fresh id.
func newSession(c models.CreateSession) *models.Session {
	now := time.Now()

	return &models.Session{
		ID:        ids.New(),
		Token:     c.Token,
		UserID:    ptrOrNil(c.UserID),
		GameID:    ptrOrNil(c.GameID),
		TeamID:    ptrOrNil(c.TeamID),
		SlotID:    ptrOrNil(c.SlotID),
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// applyUpdate copies u's non-empty fields onto sess.
func applyUpdate(sess *models.Session, u *models.UpdateSession) {
	if u.GameID != "" {
		sess.GameID = ptrOrNil(u.GameID)
	}
	if u.UserID != "" {
		sess.UserID = ptrOrNil(u.UserID)
	}
	if u.TeamID != "" {
		sess.TeamID = ptrOrNil(u.TeamID)
	}
	if u.SlotID != "" {
		sess.SlotID = ptrOrNil(u.SlotID)
	}
	sess.UpdatedAt = time.Now()
}

func ptrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
