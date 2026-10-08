package ws

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	coderws "github.com/coder/websocket"

	"github.com/robbiebyrd/indri/internal/transport"
)

// bindingServer runs a transport whose clients bind their own socket to a
// session by sending "bind:<id>", and unbind it with "unbind" — standing in for
// login and logout, which bind through the same Conn.Set.
func bindingServer(t *testing.T, loginTimeout time.Duration) (*Transport, string) {
	t.Helper()

	tr := New()
	tr.loginTimeout = loginTimeout
	tr.Handle(transport.Handlers{Message: func(c transport.Conn, msg []byte) {
		switch m := string(msg); {
		case strings.HasPrefix(m, "bind:"):
			c.Set(transport.SessionIDKey, strings.TrimPrefix(m, "bind:"))
		case m == "unbind":
			c.UnSet(transport.SessionIDKey)
		}

		// An answer, so the client knows the message has been handled.
		_ = c.Write([]byte("ok"))
	}})

	mux := http.NewServeMux()
	tr.Register(mux)

	server := httptest.NewServer(mux)
	t.Cleanup(func() {
		server.Close()
		_ = tr.Close()
	})

	return tr, "ws" + strings.TrimPrefix(server.URL, "http") + path
}

func dial(t *testing.T, url string) *coderws.Conn {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, _, err := coderws.Dial(ctx, url, nil)
	if err != nil {
		t.Fatalf("dialing: %v", err)
	}

	t.Cleanup(func() { _ = c.CloseNow() })

	return c
}

// send writes msg and waits for the server's "ok", so the binding has happened
// by the time it returns.
func send(t *testing.T, c *coderws.Conn, msg string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.Write(ctx, coderws.MessageText, []byte(msg)); err != nil {
		t.Fatalf("sending %q: %v", msg, err)
	}

	if _, got, err := c.Read(ctx); err != nil || string(got) != "ok" {
		t.Fatalf("answer to %q = %q, %v; want ok", msg, got, err)
	}
}

// closeStatus reads until the server closes c, and returns the code it closed
// with, or -1 if it is still open after wait.
func closeStatus(c *coderws.Conn, wait time.Duration) coderws.StatusCode {
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()

	for {
		if _, _, err := c.Read(ctx); err != nil {
			return coderws.CloseStatus(err)
		}
	}
}

// stillOpen reports whether c can still exchange a message with the server.
func stillOpen(c *coderws.Conn) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := c.Write(ctx, coderws.MessageText, []byte("ping")); err != nil {
		return false
	}

	_, got, err := c.Read(ctx)

	return err == nil && string(got) == "ok"
}

// A user holds one WebSocket. Binding a second socket to the same session
// closes the first with StatusSuperseded, which tells that client not to redial
// and kick the newer one back; other users' sockets are untouched.
func TestOneSocketPerUser_ANewBindingReplacesTheOld(t *testing.T) {
	_, url := bindingServer(t, time.Minute)

	older, newer, other := dial(t, url), dial(t, url), dial(t, url)

	send(t, older, "bind:alice")
	send(t, other, "bind:bob")
	send(t, newer, "bind:alice")

	if got := closeStatus(older, 3*time.Second); got != StatusSuperseded {
		t.Errorf("the older socket closed with %v, want %v", got, StatusSuperseded)
	}

	if !stillOpen(newer) {
		t.Error("the newer socket was closed; it is the one that keeps the session")
	}

	if !stillOpen(other) {
		t.Error("another user's socket was closed")
	}
}

// Binding the same socket to its session again — every login, reconnect, join
// and create answer re-binds it — must not close it.
func TestOneSocketPerUser_RebindingTheSameSocketKeepsIt(t *testing.T) {
	_, url := bindingServer(t, time.Minute)

	c := dial(t, url)

	send(t, c, "bind:alice")
	send(t, c, "bind:alice")

	if !stillOpen(c) {
		t.Error("re-binding a socket to the session it already holds closed it")
	}
}

// Sockets binding to one session at the same moment must not close each other
// out: exactly one keeps it.
func TestOneSocketPerUser_ConcurrentBindingsLeaveExactlyOne(t *testing.T) {
	_, url := bindingServer(t, time.Minute)

	const sockets = 8

	conns := make([]*coderws.Conn, sockets)
	for i := range conns {
		conns[i] = dial(t, url)
	}

	var wg sync.WaitGroup

	for _, c := range conns {
		wg.Add(1)

		go func() {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			_ = c.Write(ctx, coderws.MessageText, []byte("bind:alice"))
		}()
	}

	wg.Wait()

	open := 0

	for _, c := range conns {
		if closeStatus(c, time.Second) == -1 {
			open++
		}
	}

	if open != 1 {
		t.Errorf("%d of %d sockets bound to one session are still open, want exactly 1", open, sockets)
	}
}

// A socket that never logs in is closed with StatusLoginDeadline once the
// deadline passes; it holds memory an unauthenticated client gave nothing for.
func TestLoginDeadline_AnAnonymousSocketIsClosed(t *testing.T) {
	_, url := bindingServer(t, 100*time.Millisecond)

	c := dial(t, url)

	if got := closeStatus(c, 3*time.Second); got != StatusLoginDeadline {
		t.Errorf("an anonymous socket closed with %v, want %v", got, StatusLoginDeadline)
	}
}

// A socket that logs in in time keeps going past the deadline.
func TestLoginDeadline_ABoundSocketOutlivesIt(t *testing.T) {
	_, url := bindingServer(t, 100*time.Millisecond)

	c := dial(t, url)
	send(t, c, "bind:alice")

	time.Sleep(300 * time.Millisecond)

	if !stillOpen(c) {
		t.Error("a socket bound to a session was closed by the login deadline")
	}
}

// A socket that logs out is anonymous again, and the deadline starts over.
func TestLoginDeadline_RestartsWhenTheSocketIsUnbound(t *testing.T) {
	_, url := bindingServer(t, 200*time.Millisecond)

	c := dial(t, url)
	send(t, c, "bind:alice")

	time.Sleep(300 * time.Millisecond)
	send(t, c, "unbind")

	if got := closeStatus(c, 3*time.Second); got != StatusLoginDeadline {
		t.Errorf("an unbound socket closed with %v, want %v", got, StatusLoginDeadline)
	}
}

// A replaced socket acts on nothing more. A message its client sent after the
// replacement would otherwise still run as the user — the socket keeps its
// session key until it is gone — and a bind among them would take the session
// back from the newer socket. The library discards what arrives once the close
// frame is out; this holds it to that.
func TestOneSocketPerUser_AReplacedSocketActsOnNothingMore(t *testing.T) {
	var handled sync.Map

	tr := New()
	tr.loginTimeout = time.Minute
	tr.Handle(transport.Handlers{Message: func(c transport.Conn, msg []byte) {
		if m := string(msg); strings.HasPrefix(m, "bind:") {
			c.Set(transport.SessionIDKey, strings.TrimPrefix(m, "bind:"))
		} else {
			handled.Store(m, true)
		}

		_ = c.Write([]byte("ok"))
	}})

	mux := http.NewServeMux()
	tr.Register(mux)

	server := httptest.NewServer(mux)
	t.Cleanup(func() {
		server.Close()
		_ = tr.Close()
	})

	url := "ws" + strings.TrimPrefix(server.URL, "http") + path
	older, newer := dial(t, url), dial(t, url)

	send(t, older, "bind:alice")
	send(t, newer, "bind:alice")

	// The older client has not read its close frame yet, so its writes still
	// reach the server.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_ = older.Write(ctx, coderws.MessageText, []byte("move"))
	_ = older.Write(ctx, coderws.MessageText, []byte("bind:alice"))

	_ = closeStatus(older, 2*time.Second)

	if _, ok := handled.Load("move"); ok {
		t.Error("a message sent on a replaced socket was acted on")
	}

	if !stillOpen(newer) {
		t.Error("a bind sent on the replaced socket took the session back")
	}
}

// A handler already running on a socket when that socket is replaced must not
// bind it afterwards: a login that was mid-flight on the old socket would
// otherwise close the newer one.
func TestOneSocketPerUser_ABindingInFlightOnAReplacedSocketIsRefused(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})

	tr := New()
	tr.loginTimeout = time.Minute
	tr.Handle(transport.Handlers{Message: func(c transport.Conn, msg []byte) {
		switch m := string(msg); m {
		case "slowbind":
			close(started)
			<-release
			c.Set(transport.SessionIDKey, "alice")
		default:
			c.Set(transport.SessionIDKey, strings.TrimPrefix(m, "bind:"))
		}

		_ = c.Write([]byte("ok"))
	}})

	mux := http.NewServeMux()
	tr.Register(mux)

	server := httptest.NewServer(mux)
	t.Cleanup(func() {
		server.Close()
		_ = tr.Close()
	})

	url := "ws" + strings.TrimPrefix(server.URL, "http") + path
	older, newer := dial(t, url), dial(t, url)

	send(t, older, "bind:alice")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := older.Write(ctx, coderws.MessageText, []byte("slowbind")); err != nil {
		t.Fatalf("sending: %v", err)
	}

	<-started

	send(t, newer, "bind:alice")
	close(release)

	_ = closeStatus(older, 2*time.Second)

	if !stillOpen(newer) {
		t.Error("a binding in flight on the replaced socket closed the newer one")
	}
}
