package game

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/services/lock"
)

// TestMutate_ReturnsWhenTheCallersContextIsCancelled needs no database: the
// lock is taken before the first query, so a Mutate that is still waiting for a
// busy game has not touched Mongo yet.
//
// This is the whole point of Mutate taking a context. It used to pass the
// store's boot-time context, which is cancelled only at shutdown, so a caller
// that gave up — a client that disconnected, a scripted handler past its
// deadline — stayed queued on the game lock behind whoever held it.
func TestMutate_ReturnsWhenTheCallersContextIsCancelled(t *testing.T) {
	background := context.Background()
	locks := lock.NewInProcess()

	store := NewMemoryStore(background, locks, nil)

	// Somebody else is mid-write on this game.
	holder, err := locks.Acquire(background, "game:the-game")
	if err != nil {
		t.Fatalf("acquiring the game lock = %v, want no error", err)
	}

	defer func() { _ = holder.Release(background) }()

	ctx, cancel := context.WithCancel(background)

	applied := false
	done := make(chan error, 1)

	go func() {
		done <- store.Mutate(ctx, "the-game", func(*models.Game) error {
			applied = true

			return nil
		})
	}()

	select {
	case err := <-done:
		t.Fatalf("Mutate returned %v while the game was still locked", err)
	case <-time.After(50 * time.Millisecond):
	}

	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Mutate(cancelled ctx) = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Mutate did not return within 2s of its context being cancelled")
	}

	if applied {
		t.Error("Mutate ran apply for a caller that had already been cancelled")
	}
}
