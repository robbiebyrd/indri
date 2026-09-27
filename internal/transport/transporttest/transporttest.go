// Package transporttest is the conformance suite every transport.Transport
// must pass. It pins down the behavior the layers above the interface rely on
// (routing, broadcast targeting by sessionId, kick, shutdown), so a new
// transport is correct exactly when it passes Run.
package transporttest

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/robbiebyrd/indri/internal/transport"
)

// Timeout bounds every wait in the suite.
const Timeout = 5 * time.Second

// Frame is one server-to-client message as the client received it.
type Frame struct {
	Data   []byte
	Binary bool
}

// Client is one test connection speaking the transport's wire protocol.
type Client interface {
	// Send delivers one client-to-server message.
	Send(msg []byte) error
	// Receive returns the next server-to-client frame, or an error once the
	// connection has been closed by the server.
	Receive(timeout time.Duration) (Frame, error)
	// Close closes the connection from the client side.
	Close() error
}

// Harness adapts one transport implementation to the suite.
type Harness struct {
	// New builds a fresh, unconnected transport.
	New func(t *testing.T) transport.Transport
	// Dial opens a client connection to a server whose root URL is baseURL,
	// adding query (e.g. "debug=1", or "") to the request that opens it.
	// It returns once the transport would have fired Connect.
	Dial func(t *testing.T, baseURL string, query string) Client
}

// greeting is written from the Connect handler, the way entrypoints.HandleConnect
// writes the login scene; a transport must not fire Connect before the client
// can receive it.
const greeting = "hello"

// server wires a transport to an HTTP server with recording handlers.
type server struct {
	t        transport.Transport
	url      string
	messages chan received
	connects chan transport.Conn

	disconnectsMu sync.Mutex
	disconnects   map[transport.Conn]int
	// debugAtConnect records each conn's debug flag at the moment Connect fired.
	debugAtConnect map[transport.Conn]bool
	// openAtDisconnect counts Disconnects fired while the conn still reported
	// open; HandleDisconnect would then write to and re-close a dead conn.
	openAtDisconnect int

	inFlight, maxInFlight atomic.Int32
}

type received struct {
	conn transport.Conn
	msg  []byte
}

func start(t *testing.T, h Harness) *server {
	t.Helper()

	s := &server{
		t:              h.New(t),
		messages:       make(chan received, 1024),
		connects:       make(chan transport.Conn, 64),
		disconnects:    map[transport.Conn]int{},
		debugAtConnect: map[transport.Conn]bool{},
	}

	s.t.Handle(transport.Handlers{
		Connect: func(c transport.Conn) {
			debug, _ := c.Get("debug")
			s.disconnectsMu.Lock()
			s.debugAtConnect[c] = debug == true
			s.disconnectsMu.Unlock()

			if err := c.Write([]byte(greeting)); err != nil {
				t.Errorf("greeting write: %v", err)
			}
			s.connects <- c
		},
		Disconnect: func(c transport.Conn) {
			s.disconnectsMu.Lock()
			s.disconnects[c]++
			if !c.IsClosed() {
				s.openAtDisconnect++
			}
			s.disconnectsMu.Unlock()
		},
		Message: func(c transport.Conn, msg []byte) {
			n := s.inFlight.Add(1)
			for {
				m := s.maxInFlight.Load()
				if n <= m || s.maxInFlight.CompareAndSwap(m, n) {
					break
				}
			}
			time.Sleep(200 * time.Microsecond)
			s.inFlight.Add(-1)

			// "id:<x>" tags the connection the way login/reconnect set the
			// sessionId key; broadcast targeting filters on it.
			if bytes.HasPrefix(msg, []byte("id:")) {
				c.Set("sessionId", string(msg[3:]))
			}
			s.messages <- received{c, append([]byte(nil), msg...)}
		},
	})

	mux := http.NewServeMux()
	s.t.Register(mux)
	srv := httptest.NewServer(mux)
	s.url = srv.URL

	t.Cleanup(func() {
		if !s.t.IsClosed() {
			_ = s.t.Close()
		}
		srv.Close()
	})

	return s
}

// connect dials a client, consumes the greeting, and returns the client with
// its server-side conn.
func (s *server) connect(t *testing.T, h Harness) (Client, transport.Conn) {
	t.Helper()

	return s.connectWith(t, h, "")
}

func (s *server) connectWith(t *testing.T, h Harness, query string) (Client, transport.Conn) {
	t.Helper()

	c := h.Dial(t, s.url, query)
	t.Cleanup(func() { _ = c.Close() })

	var conn transport.Conn
	select {
	case conn = <-s.connects:
	case <-time.After(Timeout):
		t.Fatal("Connect never fired")
	}

	f, err := c.Receive(Timeout)
	if err != nil {
		t.Fatalf("receiving greeting: %v", err)
	}
	if f.Binary || string(f.Data) != greeting {
		t.Fatalf("first frame = %+v, want text %q", f, greeting)
	}

	return c, conn
}

func (s *server) next(t *testing.T) received {
	t.Helper()

	select {
	case r := <-s.messages:
		return r
	case <-time.After(Timeout):
		t.Fatal("no message reached the Message handler")
		return received{}
	}
}

func (s *server) disconnectCount(c transport.Conn) int {
	s.disconnectsMu.Lock()
	defer s.disconnectsMu.Unlock()

	return s.disconnects[c]
}

// assertSingleClosedDisconnect waits for c's Disconnect, then checks it fired
// once and saw the conn already closed — the two things HandleDisconnect
// relies on when a kick closes a conn and the transport then reports it.
func (s *server) assertSingleClosedDisconnect(t *testing.T, c transport.Conn) {
	t.Helper()

	eventually(t, "Disconnect", func() bool { return s.disconnectCount(c) > 0 })

	time.Sleep(100 * time.Millisecond)

	if n := s.disconnectCount(c); n != 1 {
		t.Fatalf("Disconnect fired %d times, want 1", n)
	}

	s.disconnectsMu.Lock()
	defer s.disconnectsMu.Unlock()

	if s.openAtDisconnect != 0 {
		t.Fatal("Disconnect fired while the conn still reported open")
	}
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(Timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Run executes the conformance suite against the harness.
func Run(t *testing.T, h Harness) {
	t.Run("connect delivers the Connect-time write", func(t *testing.T) {
		s := start(t, h)
		s.connect(t, h)
	})

	t.Run("?debug=1 marks the connection before Connect", func(t *testing.T) {
		s := start(t, h)

		_, plain := s.connect(t, h)
		_, debug := s.connectWith(t, h, "debug=1")

		// Checked as of Connect: the Connect-time write already goes
		// through WriteEncoded, so a later flag would come too late.
		s.disconnectsMu.Lock()
		defer s.disconnectsMu.Unlock()

		if s.debugAtConnect[plain] {
			t.Fatal("a connection without ?debug=1 was marked debug")
		}
		if !s.debugAtConnect[debug] {
			t.Fatal("?debug=1 was not set by the time Connect fired")
		}
	})

	t.Run("text and binary writes arrive intact with their kind", func(t *testing.T) {
		s := start(t, h)
		c, conn := s.connect(t, h)

		text := []byte(`{"stage":{"currentScene":"login"}}`)
		binary := []byte{0x00, 0x81, 0xa1, 0xff, '\n', 0xc0}

		if err := conn.Write(text); err != nil {
			t.Fatal(err)
		}
		if err := conn.WriteBinary(binary); err != nil {
			t.Fatal(err)
		}

		f, err := c.Receive(Timeout)
		if err != nil || f.Binary || !bytes.Equal(f.Data, text) {
			t.Fatalf("text frame = %+v, %v", f, err)
		}

		f, err = c.Receive(Timeout)
		if err != nil || !f.Binary || !bytes.Equal(f.Data, binary) {
			t.Fatalf("binary frame = %+v, %v", f, err)
		}
	})

	t.Run("messages from one client are delivered in order, one at a time", func(t *testing.T) {
		s := start(t, h)
		c, conn := s.connect(t, h)

		const n = 30
		for i := 0; i < n; i++ {
			if err := c.Send([]byte(fmt.Sprintf("m%d", i))); err != nil {
				t.Fatal(err)
			}
		}

		for i := 0; i < n; i++ {
			r := s.next(t)
			if r.conn != conn {
				t.Fatal("message attributed to the wrong conn")
			}
			if want := fmt.Sprintf("m%d", i); string(r.msg) != want {
				t.Fatalf("message %d = %q, want %q", i, r.msg, want)
			}
		}

		if m := s.maxInFlight.Load(); m != 1 {
			t.Fatalf("Message ran %d-way concurrently for one conn; handlers assume serial delivery", m)
		}
	})

	t.Run("concurrent writes to one conn all arrive", func(t *testing.T) {
		s := start(t, h)
		c, conn := s.connect(t, h)

		const n = 50
		var wg sync.WaitGroup
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := conn.Write([]byte("w")); err != nil {
					t.Errorf("write: %v", err)
				}
			}()
		}
		wg.Wait()

		for i := 0; i < n; i++ {
			if _, err := c.Receive(Timeout); err != nil {
				t.Fatalf("received %d of %d: %v", i, n, err)
			}
		}
	})

	t.Run("BroadcastFilter targets by sessionId; Broadcast reaches all", func(t *testing.T) {
		s := start(t, h)
		a, _ := s.connect(t, h)
		b, _ := s.connect(t, h)

		if err := a.Send([]byte("id:A")); err != nil {
			t.Fatal(err)
		}
		s.next(t)
		if err := b.Send([]byte("id:B")); err != nil {
			t.Fatal(err)
		}
		s.next(t)

		err := s.t.BroadcastFilter([]byte("for-a"), func(c transport.Conn) bool {
			v, _ := c.Get("sessionId")
			return v == "A"
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.t.Broadcast([]byte("for-all")); err != nil {
			t.Fatal(err)
		}

		for _, want := range []string{"for-a", "for-all"} {
			f, err := a.Receive(Timeout)
			if err != nil || string(f.Data) != want {
				t.Fatalf("a got %q, %v; want %q", f.Data, err, want)
			}
		}

		// b's next frame must be the broadcast: the filtered frame never came.
		f, err := b.Receive(Timeout)
		if err != nil || string(f.Data) != "for-all" {
			t.Fatalf("b got %q, %v; want %q", f.Data, err, "for-all")
		}
	})

	t.Run("server Close (kick) disconnects exactly once and deregisters", func(t *testing.T) {
		s := start(t, h)
		c, conn := s.connect(t, h)

		// Kick writes {"disconnected": true} and then closes; the notice
		// must still reach the client.
		if err := conn.Write([]byte("bye")); err != nil {
			t.Fatal(err)
		}
		if err := conn.Close(); err != nil {
			t.Fatal(err)
		}

		f, err := c.Receive(Timeout)
		if err != nil || string(f.Data) != "bye" {
			t.Fatalf("write queued before Close was lost: got %q, %v", f.Data, err)
		}

		if _, err := c.Receive(Timeout); err == nil {
			t.Fatal("client still receiving after server Close")
		}

		s.assertSingleClosedDisconnect(t, conn)
		eventually(t, "conn to leave Conns()", func() bool {
			conns, _ := s.t.Conns()
			return len(conns) == 0
		})
	})

	t.Run("client close disconnects exactly once", func(t *testing.T) {
		s := start(t, h)
		c, conn := s.connect(t, h)

		if err := c.Close(); err != nil {
			t.Fatal(err)
		}

		s.assertSingleClosedDisconnect(t, conn)
	})

	t.Run("transport Close closes every conn", func(t *testing.T) {
		s := start(t, h)
		c, conn := s.connect(t, h)

		if err := s.t.Close(); err != nil {
			t.Fatal(err)
		}
		if !s.t.IsClosed() {
			t.Fatal("IsClosed false after Close")
		}

		eventually(t, "conn to close", conn.IsClosed)

		if _, err := c.Receive(Timeout); err == nil {
			t.Fatal("client still receiving after transport Close")
		}
	})
}
