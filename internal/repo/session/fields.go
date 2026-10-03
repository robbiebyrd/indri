package session

import (
	"errors"
	"fmt"
	"time"

	"github.com/robbiebyrd/indri/internal/models"
	repoErrors "github.com/robbiebyrd/indri/internal/repo"
	"github.com/robbiebyrd/indri/internal/repo/ids"
)

// Session field handling shared by every store, so each one keeps, changes,
// expires and looks up exactly the same fields.

// errNoUser refuses a session that belongs to nobody: sessions are one per
// user, and the user is what that rule is keyed on.
var errNoUser = errors.New("a session must have a user id")

// newSession builds the session a create stores, with a freshly minted id.
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

// applyUpdate copies an update's non-empty fields onto sess. An empty field
// means "leave it", so an update naming only the team keeps the game and slot.
func applyUpdate(sess *models.Session, u *models.UpdateSession) {
	for _, field := range []struct {
		value  string
		target **string
	}{
		{u.GameID, &sess.GameID},
		{u.UserID, &sess.UserID},
		{u.TeamID, &sess.TeamID},
		{u.SlotID, &sess.SlotID},
	} {
		if field.value != "" {
			*field.target = ptrOrNil(field.value)
		}
	}

	sess.UpdatedAt = time.Now()
}

// expiryCutoff is the creation time at or before which a session has expired.
func expiryCutoff() time.Time {
	return time.Now().Add(-sessionMaxAge)
}

// alive reports whether sess was created after the expiry cutoff.
func alive(sess *models.Session) bool {
	return sess.CreatedAt.After(expiryCutoff())
}

// lookups are the keys Find and FindFirst accept, and how each reads a session.
// Anything else is an error rather than an empty result, so a misspelt key
// cannot read as "nobody". The token is deliberately absent: it is a secret,
// looked up only through GetByToken.
var lookups = map[string]func(*models.Session) *string{
	"userId": func(s *models.Session) *string { return s.UserID },
	"gameId": func(s *models.Session) *string { return s.GameID },
	"teamId": func(s *models.Session) *string { return s.TeamID },
	"slotId": func(s *models.Session) *string { return s.SlotID },
}

// requireLookup rejects a key no store can look a session up by.
func requireLookup(key string) error {
	if _, ok := lookups[key]; !ok {
		return fmt.Errorf("sessions cannot be looked up by %q", key)
	}

	return nil
}

// matches reports whether sess's key field equals value. key must have passed
// requireLookup.
func matches(sess *models.Session, key string, value string) bool {
	field := lookups[key](sess)

	return field != nil && *field == value
}

// first is FindFirst over a Find result.
func first(found []*models.Session, key string, value string) (*models.Session, error) {
	if len(found) == 0 {
		return nil, fmt.Errorf("no session with %s %q: %w", key, value, repoErrors.ErrNotFound)
	}

	return found[0], nil
}

// requireToken refuses the empty token, which must never match a session that
// has none.
func requireToken(token string) error {
	if token == "" {
		return errors.New("looking up a session: the token is empty")
	}

	return nil
}

func ptrOrNil(s string) *string {
	if s == "" {
		return nil
	}

	return &s
}
