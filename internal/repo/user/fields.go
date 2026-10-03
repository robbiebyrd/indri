package user

import (
	"fmt"
	"time"

	"github.com/robbiebyrd/indri/internal/models"
	repoErrors "github.com/robbiebyrd/indri/internal/repo"
	"github.com/robbiebyrd/indri/internal/repo/ids"
)

// User field handling shared by every store, so each one keeps, changes and
// looks up exactly the same fields.

// newUser builds the user a create stores, with a freshly minted id.
func newUser(c models.CreateUser) *models.User {
	now := time.Now()

	return &models.User{
		ID:          ids.New(),
		Email:       c.Email,
		Name:        c.Name,
		DisplayName: c.DisplayName,
		Password:    c.Password,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// applyUpdate copies the fields an update sets onto u. An empty string or a nil
// pointer means "leave it", so an edit that does not mention the password
// keeps it.
func applyUpdate(u *models.User, update *models.UpdateUser) {
	if update.Email != "" {
		u.Email = update.Email
	}

	if update.Name != "" {
		u.Name = update.Name
	}

	if update.DisplayName != nil {
		u.DisplayName = update.DisplayName
	}

	if update.Password != nil {
		u.Password = update.Password
	}

	u.UpdatedAt = time.Now()
}

// lookups are the keys Find and FindFirst accept, and how each reads a user.
// Anything else is an error rather than an empty result, so a misspelt key
// cannot read as "no such user".
var lookups = map[string]func(*models.User) string{
	"email": func(u *models.User) string { return u.Email },
	"name":  func(u *models.User) string { return u.Name },
}

// requireLookup rejects a key no store can look a user up by.
func requireLookup(key string) error {
	if _, ok := lookups[key]; !ok {
		return fmt.Errorf("users cannot be looked up by %q", key)
	}

	return nil
}

// matches reports whether u's key field equals value. key must have passed
// requireLookup.
func matches(u *models.User, key string, value string) bool {
	return lookups[key](u) == value
}

// first is FindFirst over a Find result.
func first(found []*models.User, key string, value string) (*models.User, error) {
	if len(found) == 0 {
		return nil, fmt.Errorf("no user with %s %q: %w", key, value, repoErrors.ErrNotFound)
	}

	return found[0], nil
}

// requireID rejects an update that names no user.
func requireID(update *models.UpdateUser) error {
	if update == nil || update.ID == "" {
		return fmt.Errorf("updating a user: no user id given")
	}

	return nil
}
