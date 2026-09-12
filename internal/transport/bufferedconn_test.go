package transport

import (
	"sync"
	"testing"
)

func TestBufferedConn_WriteDeliversACopy(t *testing.T) {
	c := NewBufferedConn[[]byte]("a", 4)

	buf := []byte("first")
	if err := c.Write(buf); err != nil {
		t.Fatalf("Write() = %v", err)
	}

	// Broadcasters reuse their buffer; the queued value must not follow it.
	copy(buf, "SECON")

	got := <-c.Events()
	if string(got) != "first" {
		t.Errorf("received %q, want %q — the queued value aliased the caller's buffer", got, "first")
	}
}

// A subscriber that stops reading must not wedge the broadcaster, which is
// fanning the same delta out to every other player in the game.
func TestBufferedConn_WriteDropsWhenTheBufferIsFull(t *testing.T) {
	c := NewBufferedConn[[]byte]("a", 1)

	for range 10 {
		if err := c.Write([]byte("x")); err != nil {
			t.Fatalf("Write() = %v", err)
		}
	}

	if got := len(c.Events()); got != 1 {
		t.Errorf("buffered %d messages, want 1 (the rest dropped)", got)
	}
}

func TestBufferedConn_CloseIsIdempotentAndClosesTheChannel(t *testing.T) {
	c := NewBufferedConn[[]byte]("a", 1)

	if err := c.Close(); err != nil {
		t.Fatalf("Close() = %v", err)
	}

	if err := c.Close(); err != nil {
		t.Fatalf("second Close() = %v", err)
	}

	if _, open := <-c.Events(); open {
		t.Error("Events() channel is still open after Close")
	}
}

func TestBufferedConn_WriteAfterCloseIsDroppedNotPanicked(t *testing.T) {
	c := NewBufferedConn[[]byte]("a", 1)
	_ = c.Close()

	if err := c.Write([]byte("x")); err != nil {
		t.Errorf("Write() after Close = %v, want nil", err)
	}
}

// Broadcast and shutdown genuinely overlap: the broadcaster writes while the
// client's request context is cancelling. Run under -race.
func TestBufferedConn_ConcurrentWriteAndCloseIsSafe(t *testing.T) {
	for range 200 {
		c := NewBufferedConn[[]byte]("a", 1)

		var wg sync.WaitGroup

		wg.Add(3)

		go func() { defer wg.Done(); _ = c.Write([]byte("x")) }()
		go func() { defer wg.Done(); _ = c.Write([]byte("y")) }()
		go func() { defer wg.Done(); _ = c.Close() }()

		wg.Wait()
	}
}
