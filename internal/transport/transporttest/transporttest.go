// Package transporttest is the conformance suite a transport.Transport must
// pass. It pins down what the layers above the interface rely on — that a
// connection registers and can be written to, that broadcasts are targeted by
// the sessionId key, that a kick or a client hang-up ends a connection exactly
// once, and that closing the transport ends them all — so a transport is
// correct exactly when it passes Run.
//
// Two kinds of transport hold connections. Push transports (SSE, the GraphQL
// subscription) only write to the client; the client's actions arrive over a
// separate route. Two-way transports (WebSocket, WebRTC) also carry the client's
// messages, and fire the Connect, Disconnect and Message handlers. Run checks
// the contract both share, and the parts only a two-way transport can keep
// when the harness says it is one.
//
// REST holds no connections, so it has nothing here to conform to; the parity
// tests in services/boot hold its routes to the action registry instead.
package transporttest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/robbiebyrd/indri/internal/transport"
)

// Timeout bounds every wait in the suite.
const Timeout = 5 * time.Second

// EndNotice bounds how long ending a connection may take: to deregister, to
// fire Disconnect, and for the client to see it end. It is under WebRTC's
// five-second ICE disconnect timeout, so a connection left to die without being
// told cannot pass by waiting it out.
const EndNotice = 3 * time.Second

// Client is one test connection speaking the transport's wire protocol.
type Client interface {
	// Receive returns the next message the server wrote, or an error once the
	// server has ended the connection.
	Receive(timeout time.Duration) ([]byte, error)

	// Close ends the connection from the client side.
	Close() error
}

// Sender is a Client that can also send the server a message. Only a two-way
// transport's client is one.
type Sender interface {
	Client
	Send(msg []byte) error
}

// Harness adapts one transport to the suite.
type Harness struct {
	// New builds a fresh transport.
	New func(t *testing.T) transport.Transport

	// Dial opens a client connection to the server at baseURL, authenticated as
	// sessionID where the transport authenticates at connect time. It returns
	// once the connection is established from the client's side.
	Dial func(t *testing.T, baseURL string, sessionID string) Client

	// TwoWay says the transport carries client messages and fires the Connect,
	// Disconnect and Message handlers. Its clients must be Senders.
	TwoWay bool
}

// server is a transport under test, wired to an HTTP server with recording
// handlers.
type server struct {
	tr  transport.Transport
	url string

	mu          sync.Mutex
	disconnects map[transport.Conn]int
	// openAtDisconnect counts Disconnects fired while the conn still reported
	// open: a handler that wrote to it would write to a dead connection.
	openAtDisconnect int

	messages              chan []byte
	inFlight, maxInFlight atomic.Int32
}

func start(t *testing.T, h Harness) *server {
	t.Helper()

	s := &server{
		tr:          h.New(t),
		disconnects: map[transport.Conn]int{},
		messages:    make(chan []byte, 1024),
	}

	s.tr.Handle(transport.Handlers{
		Disconnect: func(c transport.Conn) {
			s.mu.Lock()
			defer s.mu.Unlock()

			s.disconnects[c]++

			if !c.IsClosed() {
				s.openAtDisconnect++
			}
		},
		Message: func(_ transport.Conn, msg []byte) {
			n := s.inFlight.Add(1)

			for {
				m := s.maxInFlight.Load()
				if n <= m || s.maxInFlight.CompareAndSwap(m, n) {
					break
				}
			}

			// Long enough that two deliveries overlapping would be seen.
			time.Sleep(200 * time.Microsecond)
			s.inFlight.Add(-1)

			s.messages <- append([]byte(nil), msg...)
		},
	})

	mux := http.NewServeMux()
	s.tr.Register(mux)
	srv := httptest.NewServer(mux)
	s.url = srv.URL

	t.Cleanup(func() {
		if !s.tr.IsClosed() {
			_ = s.tr.Close()
		}

		srv.Close()
	})

	return s
}

// connect dials a client as sessionID and returns it with its server-side
// conn, tagged with the sessionId key the way login and push authentication
// tag it.
func (s *server) connect(t *testing.T, h Harness, sessionID string) (Client, transport.Conn) {
	t.Helper()

	before := s.conns(t)
	client := h.Dial(t, s.url, sessionID)

	var conn transport.Conn

	eventually(t, "the new connection to register", func() bool {
		for _, c := range s.conns(t) {
			if !slices.Contains(before, c) {
				conn = c

				return true
			}
		}

		return false
	})

	conn.Set(transport.SessionIDKey, sessionID)

	return client, conn
}

func (s *server) conns(t *testing.T) []transport.Conn {
	t.Helper()

	conns, err := s.tr.Conns()
	if err != nil {
		t.Fatalf("Conns: %v", err)
	}

	return conns
}

func (s *server) registered(t *testing.T, c transport.Conn) bool {
	return slices.Contains(s.conns(t), c)
}

func (s *server) disconnectsOf(c transport.Conn) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.disconnects[c]
}

// assertEndedOnce checks a connection the server has ended: it has left the
// registry, its client sees it closed, and — on a two-way transport — the
// Disconnect handler ran exactly once, after the conn was marked closed.
func (s *server) assertEndedOnce(t *testing.T, h Harness, client Client, c transport.Conn) {
	t.Helper()

	within(t, EndNotice, "the connection to deregister", func() bool { return !s.registered(t, c) })

	within(t, EndNotice, "the ended connection to report closed", c.IsClosed)

	if h.TwoWay {
		within(t, EndNotice, "Disconnect to fire", func() bool { return s.disconnectsOf(c) > 0 })

		// Give a duplicate Disconnect the chance to arrive before counting.
		time.Sleep(50 * time.Millisecond)

		if n := s.disconnectsOf(c); n != 1 {
			t.Errorf("Disconnect fired %d times, want exactly once", n)
		}

		s.mu.Lock()
		open := s.openAtDisconnect
		s.mu.Unlock()

		if open != 0 {
			t.Errorf("Disconnect fired %d times while the conn still reported open", open)
		}
	}

	if client != nil {
		assertClosedForClient(t, client)
	}
}

// assertClosedForClient drains anything still in flight and expects the
// client to see the connection end within EndNotice.
func assertClosedForClient(t *testing.T, client Client) {
	t.Helper()

	deadline := time.Now().Add(EndNotice)

	for time.Now().Before(deadline) {
		if _, err := client.Receive(time.Until(deadline)); err != nil {
			return
		}
	}

	t.Errorf("the client did not see its connection end within %v", EndNotice)
}

func receive(t *testing.T, client Client) []byte {
	t.Helper()

	msg, err := client.Receive(Timeout)
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}

	return msg
}

// payload is a server-to-client message. Every message the server sends is
// JSON, and the GraphQL subscription relays it as raw JSON, so the suite's
// messages are JSON strings.
func payload(s string) []byte {
	b, _ := json.Marshal(s)

	return b
}

// eventually polls cond until it holds or the suite's timeout passes.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()

	within(t, Timeout, what, cond)
}

// within polls cond until it holds or limit passes.
func within(t *testing.T, limit time.Duration, what string, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(limit)

	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("waited %v for %s", limit, what)
		}

		time.Sleep(5 * time.Millisecond)
	}
}

// Run is the conformance suite.
func Run(t *testing.T, h Harness) {
	t.Run("a connection registers and a server write reaches its client", func(t *testing.T) {
		s := start(t, h)
		client, conn := s.connect(t, h, "s1")

		if err := conn.Write(payload("hello")); err != nil {
			t.Fatalf("Write: %v", err)
		}

		if got := receive(t, client); string(got) != string(payload("hello")) {
			t.Fatalf("client received %s, want %s", got, payload("hello"))
		}
	})

	t.Run("concurrent writes to one connection all arrive intact", func(t *testing.T) {
		s := start(t, h)
		client, conn := s.connect(t, h, "s1")

		// Fewer than a push connection buffers: a push transport drops rather
		// than blocking the broadcaster, by design, once its buffer is full.
		const writers = 8

		var wg sync.WaitGroup

		for i := range writers {
			wg.Add(1)

			go func() {
				defer wg.Done()

				if err := conn.Write(payload(fmt.Sprintf("msg-%d", i))); err != nil {
					t.Errorf("Write %d: %v", i, err)
				}
			}()
		}

		wg.Wait()

		got := map[string]bool{}

		for range writers {
			got[string(receive(t, client))] = true
		}

		for i := range writers {
			if !got[string(payload(fmt.Sprintf("msg-%d", i)))] {
				t.Errorf("msg-%d never arrived (or arrived mangled): got %v", i, got)
			}
		}
	})

	t.Run("BroadcastFilter reaches only the sessions it matches; Broadcast reaches all", func(t *testing.T) {
		s := start(t, h)
		alice, _ := s.connect(t, h, "alice")
		bob, _ := s.connect(t, h, "bob")

		if err := s.tr.BroadcastFilter(payload("for-alice"), func(c transport.Conn) bool {
			id, _ := c.Get(transport.SessionIDKey)

			return id == "alice"
		}); err != nil {
			t.Fatalf("BroadcastFilter: %v", err)
		}

		if err := s.tr.Broadcast(payload("for-all")); err != nil {
			t.Fatalf("Broadcast: %v", err)
		}

		if got := receive(t, alice); string(got) != string(payload("for-alice")) {
			t.Fatalf("alice's first message = %s, want for-alice", got)
		}

		if got := receive(t, alice); string(got) != string(payload("for-all")) {
			t.Fatalf("alice's second message = %s, want for-all", got)
		}

		// Bob's first message being the broadcast proves the filtered one
		// never reached him.
		if got := receive(t, bob); string(got) != string(payload("for-all")) {
			t.Fatalf("bob's first message = %s, want for-all: the filtered message reached him", got)
		}
	})

	t.Run("a kick ends the connection exactly once", func(t *testing.T) {
		s := start(t, h)
		client, conn := s.connect(t, h, "s1")

		// A kick and a logout racing both close the conn; only one may count.
		var wg sync.WaitGroup

		for range 2 {
			wg.Add(1)

			go func() {
				defer wg.Done()

				_ = conn.Close()
			}()
		}

		wg.Wait()

		s.assertEndedOnce(t, h, client, conn)
	})

	t.Run("a client hanging up ends the connection exactly once", func(t *testing.T) {
		s := start(t, h)
		client, conn := s.connect(t, h, "s1")

		if err := client.Close(); err != nil {
			t.Fatalf("client Close: %v", err)
		}

		s.assertEndedOnce(t, h, nil, conn)
	})

	t.Run("closing the transport ends every connection", func(t *testing.T) {
		s := start(t, h)
		c1, conn1 := s.connect(t, h, "s1")
		c2, conn2 := s.connect(t, h, "s2")

		if err := s.tr.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}

		if !s.tr.IsClosed() {
			t.Error("the transport does not report closed")
		}

		s.assertEndedOnce(t, h, c1, conn1)
		s.assertEndedOnce(t, h, c2, conn2)
	})

	if !h.TwoWay {
		return
	}

	t.Run("one client's messages are delivered in order, one at a time", func(t *testing.T) {
		s := start(t, h)
		client, _ := s.connect(t, h, "s1")

		sender, ok := client.(Sender)
		if !ok {
			t.Fatalf("a two-way transport's client %T cannot send", client)
		}

		const messages = 20

		for i := range messages {
			if err := sender.Send(fmt.Appendf(nil, "%02d", i)); err != nil {
				t.Fatalf("Send %d: %v", i, err)
			}
		}

		for i := range messages {
			select {
			case got := <-s.messages:
				if want := fmt.Appendf(nil, "%02d", i); !bytes.Equal(got, want) {
					t.Fatalf("message %d = %q, want %q: delivered out of order", i, got, want)
				}
			case <-time.After(Timeout):
				t.Fatalf("message %d never arrived", i)
			}
		}

		if n := s.maxInFlight.Load(); n > 1 {
			t.Errorf("%d messages from one client were handled at once, want one at a time", n)
		}
	})
}
