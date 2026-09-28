package transport_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/robbiebyrd/indri/internal/transport"
)

type fakeTransport struct {
	path      string
	handled   bool
	conns     []transport.Conn
	sent      [][]byte
	closeErr  error
	sendErr   error
	connsErr  error
	closed    bool
	closeRuns int
}

func (f *fakeTransport) Handle(transport.Handlers) { f.handled = true }

func (f *fakeTransport) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET "+f.path, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
}

func (f *fakeTransport) Broadcast(msg []byte) error {
	return f.BroadcastFilter(msg, func(transport.Conn) bool { return true })
}

func (f *fakeTransport) BroadcastFilter(msg []byte, _ func(transport.Conn) bool) error {
	if f.sendErr != nil {
		return f.sendErr
	}

	f.sent = append(f.sent, msg)

	return nil
}

func (f *fakeTransport) Conns() ([]transport.Conn, error) { return f.conns, f.connsErr }

func (f *fakeTransport) Close() error {
	f.closeRuns++
	if f.closeErr != nil {
		return f.closeErr
	}

	f.closed = true

	return nil
}

func (f *fakeTransport) IsClosed() bool { return f.closed }

func TestComposite_HandleAndRegisterReachEveryChild(t *testing.T) {
	a := &fakeTransport{path: "/a"}
	b := &fakeTransport{path: "/b"}
	c := transport.Composite(a, b)

	c.Handle(transport.Handlers{})

	if !a.handled || !b.handled {
		t.Fatalf("Handle reached a=%v b=%v", a.handled, b.handled)
	}

	mux := http.NewServeMux()
	c.Register(mux)

	for _, p := range []string{"/a", "/b"} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, p, nil))

		if w.Code != http.StatusTeapot {
			t.Errorf("route %v not mounted (status %d)", p, w.Code)
		}
	}
}

func TestComposite_BroadcastReachesAllAndJoinsErrors(t *testing.T) {
	failing := errors.New("child down")
	a := &fakeTransport{sendErr: failing}
	b := &fakeTransport{}
	c := transport.Composite(a, b)

	err := c.Broadcast([]byte("x"))
	if !errors.Is(err, failing) {
		t.Fatalf("Broadcast err = %v, want it to wrap the failing child's error", err)
	}
	if len(b.sent) != 1 {
		t.Fatal("a failing child stopped the broadcast reaching its sibling")
	}

	err = c.BroadcastFilter([]byte("y"), func(transport.Conn) bool { return true })
	if !errors.Is(err, failing) || len(b.sent) != 2 {
		t.Fatalf("BroadcastFilter err = %v, sibling sends = %d", err, len(b.sent))
	}
}

func TestComposite_ConnsConcatenatesAndKeepsPartialResults(t *testing.T) {
	x, y, z := transport.NewQueuedConn(1, nil), transport.NewQueuedConn(1, nil), transport.NewQueuedConn(1, nil)
	failing := errors.New("cannot list")

	c := transport.Composite(
		&fakeTransport{conns: []transport.Conn{x, y}},
		&fakeTransport{connsErr: failing},
		&fakeTransport{conns: []transport.Conn{z}},
	)

	conns, err := c.Conns()
	if !errors.Is(err, failing) {
		t.Fatalf("Conns err = %v", err)
	}
	if len(conns) != 3 {
		t.Fatalf("Conns = %d, want the 3 from healthy children", len(conns))
	}
}

func TestComposite_CloseSkipsClosedChildrenAndIsClosedNeedsAll(t *testing.T) {
	failing := errors.New("stuck")
	a := &fakeTransport{}
	b := &fakeTransport{closeErr: failing}
	c := transport.Composite(a, b)

	if c.IsClosed() {
		t.Fatal("IsClosed true before Close")
	}

	if err := c.Close(); !errors.Is(err, failing) {
		t.Fatalf("Close err = %v", err)
	}
	if c.IsClosed() {
		t.Fatal("IsClosed true while a child is still open")
	}

	// Shutdown calls Close twice; the already-closed child must not be closed
	// again (melody returns ErrClosed on a second close).
	b.closeErr = nil
	if err := c.Close(); err != nil {
		t.Fatalf("second Close err = %v", err)
	}
	if a.closeRuns != 1 {
		t.Fatalf("closed child re-closed: %d runs", a.closeRuns)
	}
	if !c.IsClosed() {
		t.Fatal("IsClosed false after every child closed")
	}
}
