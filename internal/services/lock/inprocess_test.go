package lock

import (
	"context"
	"errors"
	"testing"
	"time"
)

// waitFor is how long a test allows an operation that should finish at once.
// Generous enough not to flake on a loaded CI runner, short enough that a real
// hang fails the test instead of the suite timing out.
const waitFor = 2 * time.Second

// keyCount reports how many keys the manager is tracking. A key survives only
// while someone holds or waits for it, so this is the leak check.
func keyCount(m *InProcess) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return len(m.locks)
}

// acquire runs Acquire in the background and reports its outcome on a channel,
// so a test can assert that it returned rather than blocking forever.
func acquire(ctx context.Context, m *InProcess, key string) <-chan result {
	out := make(chan result, 1)

	go func() {
		h, err := m.Acquire(ctx, key)
		out <- result{handle: h, err: err}
	}()

	return out
}

type result struct {
	handle Handle
	err    error
}

func awaitResult(t *testing.T, ch <-chan result, what string) result {
	t.Helper()

	select {
	case r := <-ch:
		return r
	case <-time.After(waitFor):
		t.Fatalf("%s did not return within %v", what, waitFor)

		return result{}
	}
}

func TestInProcessAcquire_SerializesHoldersOfTheSameKey(t *testing.T) {
	m := NewInProcess()
	ctx := context.Background()

	first, err := m.Acquire(ctx, "game:1")
	if err != nil {
		t.Fatalf("Acquire(game:1) = %v, want no error", err)
	}

	second := acquire(ctx, m, "game:1")

	select {
	case <-second:
		t.Fatal("a second Acquire of a held key returned while the first holder still had it")
	case <-time.After(50 * time.Millisecond):
	}

	if err := first.Release(ctx); err != nil {
		t.Fatalf("Release(game:1) = %v, want no error", err)
	}

	r := awaitResult(t, second, "the waiting Acquire")
	if r.err != nil {
		t.Fatalf("the waiting Acquire = %v, want it to take the released lock", r.err)
	}

	if err := r.handle.Release(ctx); err != nil {
		t.Fatalf("Release(game:1) = %v, want no error", err)
	}

	if got := keyCount(m); got != 0 {
		t.Errorf("manager still tracks %d keys after every holder released, want 0", got)
	}
}

func TestInProcessAcquire_DoesNotSerializeDifferentKeys(t *testing.T) {
	m := NewInProcess()
	ctx := context.Background()

	first, err := m.Acquire(ctx, "game:1")
	if err != nil {
		t.Fatalf("Acquire(game:1) = %v, want no error", err)
	}

	r := awaitResult(t, acquire(ctx, m, "game:2"), "Acquire(game:2)")
	if r.err != nil {
		t.Fatalf("Acquire(game:2) = %v, want a different key not to wait on game:1", r.err)
	}

	_ = r.handle.Release(ctx)
	_ = first.Release(ctx)
}

// TestInProcessAcquire_RejectsAnAlreadyCancelledContext is the cheap half of
// cancellation: no wait has started, so nothing has to be handed off.
func TestInProcessAcquire_RejectsAnAlreadyCancelledContext(t *testing.T) {
	m := NewInProcess()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	h, err := m.Acquire(ctx, "game:1")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Acquire(cancelled ctx) = %v, want context.Canceled", err)
	}

	if h != nil {
		t.Fatal("Acquire(cancelled ctx) still returned a handle")
	}

	if got := keyCount(m); got != 0 {
		t.Errorf("a rejected Acquire left %d keys behind, want 0", got)
	}
}

// TestInProcessAcquire_UnblocksWhenTheContextIsCancelledWhileWaiting is the
// case that used to hang forever: the ctx check ran once, up front, and the
// wait on the keyed mutex could not be interrupted. A caller whose deadline
// expired stayed parked on the lock until the holder happened to release.
func TestInProcessAcquire_UnblocksWhenTheContextIsCancelledWhileWaiting(t *testing.T) {
	m := NewInProcess()

	holder, err := m.Acquire(context.Background(), "game:1")
	if err != nil {
		t.Fatalf("Acquire(game:1) = %v, want no error", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	waiter := acquire(ctx, m, "game:1")

	// Let the waiter reach the mutex, so cancellation interrupts a wait in
	// progress rather than being caught by the check at the top of Acquire.
	select {
	case r := <-waiter:
		t.Fatalf("Acquire(held key) returned %v before the holder released", r.err)
	case <-time.After(50 * time.Millisecond):
	}

	cancel()

	r := awaitResult(t, waiter, "the cancelled Acquire")
	if !errors.Is(r.err, context.Canceled) {
		t.Fatalf("the cancelled Acquire = %v, want context.Canceled", r.err)
	}

	if r.handle != nil {
		t.Fatal("the cancelled Acquire still returned a handle")
	}

	_ = holder.Release(context.Background())
}

// TestInProcessAcquire_ReleasesTheLockTheAbandonedWaiterInherits is the other
// half of that fix, and the one that is easy to get wrong: the goroutine doing
// the blocking wait is still queued for the mutex after its caller has gone. If
// it takes the lock and nobody releases it, the key is dead for everyone — a
// worse outcome than the original hang, because it never recovers.
func TestInProcessAcquire_ReleasesTheLockTheAbandonedWaiterInherits(t *testing.T) {
	m := NewInProcess()
	background := context.Background()

	holder, err := m.Acquire(background, "game:1")
	if err != nil {
		t.Fatalf("Acquire(game:1) = %v, want no error", err)
	}

	ctx, cancel := context.WithCancel(background)
	abandoned := acquire(ctx, m, "game:1")

	select {
	case <-abandoned:
		t.Fatal("Acquire(held key) returned before the holder released")
	case <-time.After(50 * time.Millisecond):
	}

	cancel()

	if r := awaitResult(t, abandoned, "the cancelled Acquire"); !errors.Is(r.err, context.Canceled) {
		t.Fatalf("the cancelled Acquire = %v, want context.Canceled", r.err)
	}

	// The holder releasing hands the lock to the abandoned waiter, which must
	// give it straight back rather than keep it.
	if err := holder.Release(background); err != nil {
		t.Fatalf("Release(game:1) = %v, want no error", err)
	}

	r := awaitResult(t, acquire(background, m, "game:1"), "Acquire after the abandoned waiter")
	if r.err != nil {
		t.Fatalf("Acquire after the abandoned waiter = %v, want the key to be free", r.err)
	}

	if err := r.handle.Release(background); err != nil {
		t.Fatalf("Release(game:1) = %v, want no error", err)
	}

	// Every reference is gone, including the abandoned waiter's, so the key
	// must be off the map: a reference that is never dropped grows it forever.
	deadline := time.Now().Add(waitFor)
	for keyCount(m) != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	if got := keyCount(m); got != 0 {
		t.Errorf("manager still tracks %d keys, want 0 — the abandoned waiter leaked its reference", got)
	}
}

// TestInProcessAcquire_ReleaseIsIdempotent keeps a double Release (a deferred
// one plus an explicit one) from unlocking a mutex the caller no longer holds.
func TestInProcessAcquire_ReleaseIsIdempotent(t *testing.T) {
	m := NewInProcess()
	ctx := context.Background()

	h, err := m.Acquire(ctx, "game:1")
	if err != nil {
		t.Fatalf("Acquire(game:1) = %v, want no error", err)
	}

	if err := h.Release(ctx); err != nil {
		t.Fatalf("first Release = %v, want no error", err)
	}

	if err := h.Release(ctx); err != nil {
		t.Fatalf("second Release = %v, want no error", err)
	}

	if got := keyCount(m); got != 0 {
		t.Errorf("manager tracks %d keys after a double release, want 0", got)
	}
}
