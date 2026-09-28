// Package ids generates the canonical string identifier used across every
// backend. New() returns a UUID v4 formatted with dashes.
package ids

import "github.com/google/uuid"

// New returns a fresh v4 UUID as a lowercase, dash-formatted string.
func New() string {
	return uuid.NewString()
}
