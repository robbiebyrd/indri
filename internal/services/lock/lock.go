// Package lock provides distributed mutual exclusion behind a small interface,
// so conflict serialization lives above the database and works across
// processes. Swapping the data store never touches this layer.
package lock

import "context"

// Manager hands out exclusive locks keyed by an arbitrary string. Acquire
// blocks until the lock is held or ctx is cancelled.
type Manager interface {
	Acquire(ctx context.Context, key string) (Handle, error)
}

// Handle is a held lock. Release must be called exactly once, ideally via
// defer, to free the lock for other holders.
type Handle interface {
	Release(ctx context.Context) error
}
