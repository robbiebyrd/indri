package transport

import (
	"sync"
	"testing"
)

func TestKeys_SessionIDIsSetOnConstruction(t *testing.T) {
	k := NewKeys("abc123")

	got, ok := k.Get(SessionIDKey)
	if !ok {
		t.Fatalf("Get(%q) reported the key as absent", SessionIDKey)
	}

	if got != "abc123" {
		t.Errorf("Get(%q) = %v, want %q", SessionIDKey, got, "abc123")
	}
}

func TestKeys_SetAndUnSet(t *testing.T) {
	k := NewKeys("abc123")

	k.Set("team", "red")

	if got, ok := k.Get("team"); !ok || got != "red" {
		t.Errorf("Get(\"team\") = %v, %v, want \"red\", true", got, ok)
	}

	k.UnSet("team")

	if _, ok := k.Get("team"); ok {
		t.Error("Get(\"team\") still reports the key as present after UnSet")
	}
}

// MarkClosed reports whether the caller is the one that closed, so an embedder
// can close its channel or stream exactly once. Closing a channel twice panics.
func TestKeys_MarkClosedIsWonByExactlyOneCaller(t *testing.T) {
	k := NewKeys("abc123")

	if k.IsClosed() {
		t.Fatal("a new Keys reports itself as closed")
	}

	const callers = 50

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		wins int
	)

	wg.Add(callers)

	for range callers {
		go func() {
			defer wg.Done()

			if k.MarkClosed() {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	if wins != 1 {
		t.Errorf("MarkClosed returned true %d times, want exactly 1", wins)
	}

	if !k.IsClosed() {
		t.Error("IsClosed() = false after MarkClosed")
	}
}

func TestKeys_WhileOpenReportsWhetherItRan(t *testing.T) {
	k := NewKeys("abc123")

	ran := false
	if !k.WhileOpen(func() { ran = true }) || !ran {
		t.Error("WhileOpen did not run fn on an open connection")
	}

	k.MarkClosed()

	ran = false
	if k.WhileOpen(func() { ran = true }) || ran {
		t.Error("WhileOpen ran fn on a closed connection")
	}
}

// This is the invariant the whole type exists for: a delivery in flight must
// not overlap the Close that releases the channel it delivers to, or the send
// panics on a closed channel. Run under -race.
func TestKeys_WhileOpenNeverOverlapsMarkClosed(t *testing.T) {
	for range 200 {
		k := NewKeys("abc123")
		ch := make(chan int, 1)

		var wg sync.WaitGroup

		wg.Add(2)

		go func() {
			defer wg.Done()

			k.WhileOpen(func() {
				select {
				case ch <- 1:
				default:
				}
			})
		}()

		go func() {
			defer wg.Done()

			if k.MarkClosed() {
				close(ch)
			}
		}()

		wg.Wait()
	}
}

func TestKeys_ConcurrentAccessIsRaceFree(t *testing.T) {
	k := NewKeys("abc123")

	var wg sync.WaitGroup

	wg.Add(3)

	go func() {
		defer wg.Done()
		for range 200 {
			k.Set("n", 1)
		}
	}()
	go func() {
		defer wg.Done()
		for range 200 {
			k.Get("n")
		}
	}()
	go func() {
		defer wg.Done()
		for range 200 {
			k.IsClosed()
		}
	}()

	wg.Wait()
}
