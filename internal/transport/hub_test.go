package transport_test

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/robbiebyrd/indri/internal/transport"
)

func TestQueuedConn_KeysAreConcurrencySafe(t *testing.T) {
	c := transport.NewQueuedConn(4, nil)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Set("k", i)
			c.Get("k")
			c.UnSet("k")
		}()
	}
	wg.Wait()

	c.Set("sessionId", "abc")
	if v, ok := c.Get("sessionId"); !ok || v != "abc" {
		t.Fatalf("Get = %v, %v", v, ok)
	}

	c.UnSet("sessionId")
	if _, ok := c.Get("sessionId"); ok {
		t.Fatal("key still present after UnSet")
	}
}

func TestQueuedConn_WritesQueueInOrderWithKind(t *testing.T) {
	c := transport.NewQueuedConn(4, nil)

	if err := c.Write([]byte("text")); err != nil {
		t.Fatal(err)
	}
	if err := c.WriteBinary([]byte{0x81, 0xa1}); err != nil {
		t.Fatal(err)
	}

	first := <-c.Outbound()
	second := <-c.Outbound()

	if first.Binary || string(first.Data) != "text" {
		t.Errorf("first = %+v", first)
	}
	if !second.Binary || string(second.Data) != "\x81\xa1" {
		t.Errorf("second = %+v", second)
	}
}

func TestQueuedConn_DrainFlushesQueuedFramesAndStopsOnFailure(t *testing.T) {
	c := transport.NewQueuedConn(4, nil)
	_ = c.Write([]byte("a"))
	_ = c.Write([]byte("b"))
	_ = c.Close()

	var got []string
	c.Drain(func(f transport.Frame) bool {
		got = append(got, string(f.Data))
		return true
	})

	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("drained %v, want [a b]", got)
	}

	c = transport.NewQueuedConn(4, nil)
	_ = c.Write([]byte("a"))
	_ = c.Write([]byte("b"))

	calls := 0
	c.Drain(func(transport.Frame) bool { calls++; return false })

	if calls != 1 {
		t.Fatalf("write called %d times after failing, want 1", calls)
	}
}

func TestQueuedConn_FullBufferAndClosedReturnErrors(t *testing.T) {
	c := transport.NewQueuedConn(1, nil)

	if err := c.Write([]byte("1")); err != nil {
		t.Fatal(err)
	}
	if err := c.Write([]byte("2")); !errors.Is(err, transport.ErrBufferFull) {
		t.Fatalf("second write err = %v, want ErrBufferFull", err)
	}

	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := c.Write([]byte("3")); !errors.Is(err, transport.ErrClosed) {
		t.Fatalf("write after close err = %v, want ErrClosed", err)
	}
	if err := c.WriteBinary([]byte("3")); !errors.Is(err, transport.ErrClosed) {
		t.Fatalf("binary write after close err = %v, want ErrClosed", err)
	}
}

func TestQueuedConn_CloseIsIdempotentAndSynchronous(t *testing.T) {
	var teardowns atomic.Int32
	c := transport.NewQueuedConn(1, func() { teardowns.Add(1) })

	if c.IsClosed() {
		t.Fatal("new conn reports closed")
	}

	_ = c.Close()

	// HandleDisconnect relies on IsClosed being true immediately after Close.
	if !c.IsClosed() {
		t.Fatal("IsClosed false right after Close")
	}

	_ = c.Close()

	if n := teardowns.Load(); n != 1 {
		t.Fatalf("teardown ran %d times, want 1", n)
	}

	select {
	case <-c.Done():
	default:
		t.Fatal("Done not closed after Close")
	}
}

func TestQueuedConn_DisconnectExactlyOnceAndOnlyAfterConnect(t *testing.T) {
	var connects, disconnects atomic.Int32
	h := transport.Handlers{
		Connect:    func(transport.Conn) { connects.Add(1) },
		Disconnect: func(transport.Conn) { disconnects.Add(1) },
	}

	neverConnected := transport.NewQueuedConn(1, nil)
	neverConnected.Disconnected(h)

	if disconnects.Load() != 0 {
		t.Fatal("Disconnect fired for a conn that never connected")
	}

	c := transport.NewQueuedConn(1, nil)
	c.Connected(h)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); c.Disconnected(h) }()
	}
	wg.Wait()

	if connects.Load() != 1 || disconnects.Load() != 1 {
		t.Fatalf("connects=%d disconnects=%d, want 1/1", connects.Load(), disconnects.Load())
	}
}

func TestQueuedConn_DeliverSerializesMessages(t *testing.T) {
	var inFlight, maxInFlight atomic.Int32
	h := transport.Handlers{
		Message: func(transport.Conn, []byte) {
			n := inFlight.Add(1)
			for {
				m := maxInFlight.Load()
				if n <= m || maxInFlight.CompareAndSwap(m, n) {
					break
				}
			}
			time.Sleep(time.Millisecond)
			inFlight.Add(-1)
		},
	}

	c := transport.NewQueuedConn(1, nil)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); c.Deliver(h, []byte("m")) }()
	}
	wg.Wait()

	if m := maxInFlight.Load(); m != 1 {
		t.Fatalf("max concurrent Message calls on one conn = %d, want 1", m)
	}
}

func TestHub_RegistryBroadcastAndClose(t *testing.T) {
	hub := transport.NewHub()

	var errs atomic.Int32
	hub.Handle(transport.Handlers{Error: func(transport.Conn, error) { errs.Add(1) }})

	a := transport.NewQueuedConn(4, nil)
	b := transport.NewQueuedConn(1, nil)
	a.Set("sessionId", "A")
	b.Set("sessionId", "B")

	if err := hub.Add("id-a", a); err != nil {
		t.Fatal(err)
	}
	if err := hub.Add("id-b", b); err != nil {
		t.Fatal(err)
	}

	if got, ok := hub.Lookup("id-a"); !ok || got != a {
		t.Fatal("Lookup(id-a) failed")
	}

	conns, err := hub.Conns()
	if err != nil || len(conns) != 2 {
		t.Fatalf("Conns = %d, %v", len(conns), err)
	}

	err = hub.BroadcastFilter([]byte("only-a"), func(c transport.Conn) bool {
		v, _ := c.Get("sessionId")
		return v == "A"
	})
	if err != nil {
		t.Fatal(err)
	}

	if f := <-a.Outbound(); string(f.Data) != "only-a" {
		t.Fatalf("a got %q", f.Data)
	}
	select {
	case f := <-b.Outbound():
		t.Fatalf("b got filtered-out frame %q", f.Data)
	default:
	}

	// b's buffer holds one frame; a second broadcast overflows it. A slow
	// client must not fail the broadcast for everyone else — the error goes
	// to the Error handler instead.
	if err := hub.Broadcast([]byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := hub.Broadcast([]byte("y")); err != nil {
		t.Fatalf("broadcast failed because one conn was full: %v", err)
	}
	if errs.Load() == 0 {
		t.Error("buffer overflow on one conn was not reported to the Error handler")
	}

	hub.Remove("id-b")
	if _, ok := hub.Lookup("id-b"); ok {
		t.Fatal("id-b still registered after Remove")
	}

	if err := hub.Close(); err != nil {
		t.Fatal(err)
	}
	if !hub.IsClosed() || !a.IsClosed() {
		t.Fatal("Close did not close the hub and its conns")
	}
	if err := hub.Add("late", transport.NewQueuedConn(1, nil)); !errors.Is(err, transport.ErrClosed) {
		t.Fatalf("Add after Close err = %v, want ErrClosed", err)
	}
	if err := hub.Broadcast([]byte("z")); !errors.Is(err, transport.ErrClosed) {
		t.Fatalf("Broadcast after Close err = %v, want ErrClosed", err)
	}
}

func TestNewConnectionID(t *testing.T) {
	seen := map[string]bool{}

	for i := 0; i < 100; i++ {
		id, err := transport.NewConnectionID()
		if err != nil {
			t.Fatal(err)
		}
		// 256 bits, hex-encoded: same strength as session tokens, because
		// holding a connection ID lets you send as that connection.
		if len(id) != 64 {
			t.Fatalf("len(id) = %d, want 64", len(id))
		}
		if seen[id] {
			t.Fatal("duplicate connection ID")
		}
		seen[id] = true
	}
}
