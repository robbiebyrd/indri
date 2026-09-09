// Package mutation coordinates conflict-free record edits above the data
// store. It serializes a read-modify-write under a distributed lock and
// commits behind a version fence, so a store only needs to load a document and
// save it conditionally on its version — no store-specific concurrency
// features. This is what lets the backend be swapped (Mongo, SQLite, ...)
// without reimplementing conflict resolution.
package mutation

import (
	"context"
	"errors"

	"github.com/robbiebyrd/indri/internal/services/lock"
)

// ErrAbort lets an apply function signal that no change is needed, so Run
// returns without writing.
var ErrAbort = errors.New("mutation aborted")

// ErrConflict is returned when the version fence keeps failing, i.e. writers
// kept racing past the lock (e.g. repeated lease expiry) beyond the retry
// budget.
var ErrConflict = errors.New("mutation conflict: exceeded retry budget")

const defaultRetries = 10

// Run performs a serialized, fenced read-modify-write for key.
//
//   - load returns the current document and its version.
//   - apply mutates the document in memory (return ErrAbort to skip the write).
//   - save persists the document only if the stored version still equals
//     expected, reporting whether it committed.
//
// The distributed lock makes a losing race rare; the version fence makes it
// safe when the lock's lease expires mid-mutation.
func Run[T any](
	ctx context.Context,
	mgr lock.Manager,
	key string,
	load func() (*T, int64, error),
	apply func(*T) error,
	save func(doc *T, expectedVersion int64) (committed bool, err error),
) error {
	handle, err := mgr.Acquire(ctx, key)
	if err != nil {
		return err
	}

	defer func() { _ = handle.Release(ctx) }()

	for attempt := 0; attempt < defaultRetries; attempt++ {
		doc, version, err := load()
		if err != nil {
			return err
		}

		if err := apply(doc); err != nil {
			if errors.Is(err, ErrAbort) {
				return nil
			}

			return err
		}

		committed, err := save(doc, version)
		if err != nil {
			return err
		}

		if committed {
			return nil
		}
	}

	return ErrConflict
}
