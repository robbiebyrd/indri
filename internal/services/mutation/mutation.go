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
//
// A nil error means "nothing went wrong", which covers both a committed write
// and an aborted attempt. Callers that must tell those apart — anything with a
// side effect to release only when the state actually changed — want RunResult.
func Run[T any](
	ctx context.Context,
	mgr lock.Manager,
	key string,
	load func() (*T, int64, error),
	apply func(*T) error,
	save func(doc *T, expectedVersion int64) (committed bool, err error),
) error {
	_, err := RunResult(ctx, mgr, key, load, apply, save)

	return err
}

// RunResult is Run, reporting whether the write committed.
//
// It exists because an aborted attempt and a committed write are different
// outcomes that Run cannot distinguish: both return nil. A caller that buffers
// side effects during apply — a script's queued replies and broadcasts — has to
// know which one happened, because apply may run up to defaultRetries times and
// every attempt but the committing one must be discarded.
//
// committed is false for an abort, false alongside ErrConflict or any error
// from load/apply/save, and true only when save reported a write.
func RunResult[T any](
	ctx context.Context,
	mgr lock.Manager,
	key string,
	load func() (*T, int64, error),
	apply func(*T) error,
	save func(doc *T, expectedVersion int64) (committed bool, err error),
) (bool, error) {
	handle, err := mgr.Acquire(ctx, key)
	if err != nil {
		return false, err
	}

	defer func() { _ = handle.Release(ctx) }()

	for attempt := 0; attempt < defaultRetries; attempt++ {
		doc, version, err := load()
		if err != nil {
			return false, err
		}

		if err := apply(doc); err != nil {
			if errors.Is(err, ErrAbort) {
				return false, nil
			}

			return false, err
		}

		committed, err := save(doc, version)
		if err != nil {
			return false, err
		}

		if committed {
			return true, nil
		}
	}

	return false, ErrConflict
}
