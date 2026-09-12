package transport

import (
	"net/http"
	"slices"
	"sync"
	"testing"
)

// fakeConn is a push-only connection that records what was written to it.
type fakeConn struct {
	*Keys

	mu     sync.Mutex
	writes [][]byte
}

func newFakeConn(sessionID string) *fakeConn {
	return &fakeConn{Keys: NewKeys(sessionID)}
}

func (c *fakeConn) Write(msg []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.writes = append(c.writes, slices.Clone(msg))

	return nil
}

func (c *fakeConn) Close() error {
	c.MarkClosed()

	return nil
}

func (c *fakeConn) written() [][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()

	return slices.Clone(c.writes)
}

func TestRegistry_BroadcastReachesEverySink(t *testing.T) {
	var r Registry

	a, b := newFakeConn("a"), newFakeConn("b")
	r.AddSink(a)
	r.AddSink(b)

	if err := r.Broadcast([]byte("hello")); err != nil {
		t.Fatalf("Broadcast() = %v", err)
	}

	for name, c := range map[string]*fakeConn{"a": a, "b": b} {
		if got := c.written(); len(got) != 1 || string(got[0]) != "hello" {
			t.Errorf("conn %s received %q, want one %q", name, got, "hello")
		}
	}
}

func TestRegistry_BroadcastFilterSkipsNonMatchingSinks(t *testing.T) {
	var r Registry

	a, b := newFakeConn("a"), newFakeConn("b")
	r.AddSink(a)
	r.AddSink(b)

	err := r.BroadcastFilter([]byte("hello"), func(c Conn) bool {
		id, _ := c.Get(SessionIDKey)

		return id == "a"
	})
	if err != nil {
		t.Fatalf("BroadcastFilter() = %v", err)
	}

	if got := len(a.written()); got != 1 {
		t.Errorf("matching conn received %d messages, want 1", got)
	}

	if got := len(b.written()); got != 0 {
		t.Errorf("non-matching conn received %d messages, want 0", got)
	}
}

func TestRegistry_RemovedSinkStopsReceiving(t *testing.T) {
	var r Registry

	c := newFakeConn("a")
	r.AddSink(c)
	r.RemoveSink(c)

	if err := r.Broadcast([]byte("hello")); err != nil {
		t.Fatalf("Broadcast() = %v", err)
	}

	if got := len(c.written()); got != 0 {
		t.Errorf("removed conn received %d messages, want 0", got)
	}
}

func TestRegistry_CloseClosesAndDropsEverySink(t *testing.T) {
	var r Registry

	c := newFakeConn("a")
	r.AddSink(c)

	if err := r.Close(); err != nil {
		t.Fatalf("Close() = %v", err)
	}

	if !c.IsClosed() {
		t.Error("sink was not closed by Registry.Close")
	}

	if !r.IsClosed() {
		t.Error("IsClosed() = false after Close")
	}

	conns, err := r.Conns()
	if err != nil {
		t.Fatalf("Conns() = %v", err)
	}

	if len(conns) != 0 {
		t.Errorf("Conns() returned %d conns after Close, want 0", len(conns))
	}
}

// A subscription that opens while the transport is shutting down must not be
// registered, or it lingers with nothing left to close it.
func TestRegistry_AddSinkAfterCloseIsRejectedAndClosed(t *testing.T) {
	var r Registry

	if err := r.Close(); err != nil {
		t.Fatalf("Close() = %v", err)
	}

	c := newFakeConn("a")
	r.AddSink(c)

	if !c.IsClosed() {
		t.Error("sink added after Close was left open")
	}

	conns, _ := r.Conns()
	if len(conns) != 0 {
		t.Errorf("Conns() returned %d conns, want 0", len(conns))
	}
}

func TestRegistry_DisconnectClosesOnlyNamedSessions(t *testing.T) {
	var r Registry

	target, other := newFakeConn("kick-me"), newFakeConn("keep-me")
	r.AddSink(target)
	r.AddSink(other)

	r.Disconnect([]string{"kick-me"})

	if !target.IsClosed() {
		t.Error("named session was not disconnected")
	}

	if other.IsClosed() {
		t.Error("unnamed session was disconnected")
	}
}

// fakeTransport is a Registry promoted to a full Transport, standing in for
// another protocol's transport inside a Multi.
type fakeTransport struct{ *Registry }

func (fakeTransport) Handle(Handlers)         {}
func (fakeTransport) Register(*http.ServeMux) {}

// A kicked player may be connected over a different transport, so Disconnect
// resolves connections through the aggregate when one is wired.
func TestRegistry_DisconnectReachesPeerTransport(t *testing.T) {
	local, remote := fakeTransport{&Registry{}}, fakeTransport{&Registry{}}

	onPeer := newFakeConn("kick-me")
	remote.AddSink(onPeer)

	local.SetPeer(NewMulti(local, remote))
	local.Disconnect([]string{"kick-me"})

	if !onPeer.IsClosed() {
		t.Error("connection on the peer transport was not disconnected")
	}
}
