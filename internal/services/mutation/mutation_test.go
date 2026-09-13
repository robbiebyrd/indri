package mutation_test

import (
	"context"
	"sync"
	"testing"

	"github.com/robbiebyrd/indri/internal/services/lock"
	"github.com/robbiebyrd/indri/internal/services/mutation"
)

// fakeStore models a record with a version, guarded by a mutex so its own
// bookkeeping is data-race-free, exactly like a real store's atomic write.
type fakeStore struct {
	mu      sync.Mutex
	value   int
	version int64
}

func (s *fakeStore) load() (*int, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	v := s.value

	return &v, s.version, nil
}

// saveUnconditional always overwrites — it has no fence, so only a lock can
// prevent lost updates.
func (s *fakeStore) saveUnconditional(doc *int, _ int64) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.value = *doc
	s.version++

	return true, nil
}

// saveConditional commits only when the version is unchanged — the fence.
func (s *fakeStore) saveConditional(doc *int, expected int64) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.version != expected {
		return false, nil
	}

	s.value = *doc
	s.version++

	return true, nil
}

func increment(v *int) error {
	*v = *v + 1

	return nil
}

// noopManager hands out locks that don't serialize, to exercise the fence in
// isolation.
type noopManager struct{}

func (noopManager) Acquire(context.Context, string) (lock.Handle, error) {
	return noopHandle{}, nil
}

type noopHandle struct{}

func (noopHandle) Release(context.Context) error { return nil }

// TestRun_LockSerializes shows the distributed lock alone prevents lost updates
// even when the store cannot fence (unconditional save).
func TestRun_LockSerializes(t *testing.T) {
	store := &fakeStore{}
	mgr := lock.NewInProcess()

	const n = 50

	var wg sync.WaitGroup

	for i := 0; i < n; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			_ = mutation.Run(context.Background(), mgr, "rec",
				store.load, increment, store.saveUnconditional)
		}()
	}

	wg.Wait()

	if store.value != n {
		t.Fatalf("expected %d after serialized increments, got %d (lost updates)", n, store.value)
	}
}

// firstLoadBarrier holds every writer after its first read until all of them
// have read, then lets them all go. Without it the writers never overlap —
// each one commits before the next one looks — the fence is never asked to
// reject anything, and the test passes just as happily with the fence deleted.
type firstLoadBarrier struct {
	mu    sync.Mutex
	seen  int
	n     int
	ready chan struct{}
}

func newFirstLoadBarrier(n int) *firstLoadBarrier {
	return &firstLoadBarrier{n: n, ready: make(chan struct{})}
}

// wait blocks until n callers have arrived. A writer retrying after a rejected
// save passes straight through: it can only be retrying because it already got
// past its first load, so the barrier has already opened.
func (b *firstLoadBarrier) wait() {
	b.mu.Lock()

	select {
	case <-b.ready:
		b.mu.Unlock()

		return
	default:
	}

	b.seen++
	if b.seen == b.n {
		close(b.ready)
	}

	b.mu.Unlock()

	<-b.ready
}

// TestRun_FenceRecoversWithoutLock shows the version fence + retry prevents lost
// updates even when the lock does not serialize.
//
// Every writer loads the same version before any of them saves, so exactly one
// save commits and the rest are rejected and must retry. Drop the retry loop,
// or stop checking whether the save committed, and the count comes back as 1.
func TestRun_FenceRecoversWithoutLock(t *testing.T) {
	store := &fakeStore{}

	const n = 8

	barrier := newFirstLoadBarrier(n)

	// The barrier closes after the read, not before it: holding the writers
	// before they read only guarantees they arrive together, and the first one
	// through can still commit before the next one looks. Holding them after
	// the read is what makes every writer see the same version.
	load := func() (*int, int64, error) {
		doc, version, err := store.load()

		barrier.wait()

		return doc, version, err
	}

	var wg sync.WaitGroup

	errs := make(chan error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			if err := mutation.Run(context.Background(), noopManager{}, "rec",
				load, increment, store.saveConditional); err != nil {
				errs <- err
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("unexpected mutation error: %v", err)
	}

	if store.value != n {
		t.Fatalf("expected %d after fenced increments, got %d (lost updates)", n, store.value)
	}
}

// TestRun_Abort skips the write when apply returns ErrAbort.
func TestRun_Abort(t *testing.T) {
	store := &fakeStore{value: 7, version: 3}

	err := mutation.Run(context.Background(), lock.NewInProcess(), "rec",
		store.load,
		func(*int) error { return mutation.ErrAbort },
		store.saveConditional)
	if err != nil {
		t.Fatalf("abort should not error, got %v", err)
	}

	if store.value != 7 || store.version != 3 {
		t.Fatalf("abort should not write, got value=%d version=%d", store.value, store.version)
	}
}
