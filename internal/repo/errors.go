// Package repo defines the sentinel errors every backend maps its
// driver-specific failures onto. Callers compare with errors.Is; they never
// inspect driver types.
package repo

import "errors"

var (
	ErrNotFound  = errors.New("repo: not found")
	ErrDuplicate = errors.New("repo: duplicate key")
	ErrConflict  = errors.New("repo: version conflict")
)
