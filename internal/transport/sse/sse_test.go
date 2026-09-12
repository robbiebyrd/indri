package sse

import (
	"bufio"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/transport"
)

// fakeSessions resolves exactly one token, standing in for the session store.
type fakeSessions struct {
	token string
	id    bson.ObjectID
}

func (f fakeSessions) GetByToken(token string) (*models.Session, error) {
	if token != f.token {
		return nil, errors.New("no such session")
	}

	return &models.Session{ID: f.id, Token: token}, nil
}

// stream opens an SSE connection and returns a reader over its frames.
type stream struct {
	resp *http.Response
	r    *bufio.Reader
}

func (s *stream) close() { _ = s.resp.Body.Close() }

// line reads the next non-empty line, failing the test if none arrives. It
// bounds the wait so a hung stream fails loudly instead of blocking the suite.
func (s *stream) line(t *testing.T) string {
	t.Helper()

	type result struct {
		line string
		err  error
	}

	ch := make(chan result, 1)

	go func() {
		for {
			line, err := s.r.ReadString('\n')
			if err != nil {
				ch <- result{err: err}

				return
			}

			if trimmed := strings.TrimRight(line, "\n"); trimmed != "" {
				ch <- result{line: trimmed}

				return
			}
		}
	}()

	select {
	case res := <-ch:
		if res.err != nil {
			t.Fatalf("reading from stream: %v", res.err)
		}

		return res.line
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for a line from the stream")

		return ""
	}
}

// newTestTransport starts an HTTP server carrying only the SSE transport. The
// server mirrors the real one's WriteTimeout, which would otherwise cut every
// stream off after a few seconds.
func newTestTransport(t *testing.T, sessions SessionLookup) (*Transport, string) {
	t.Helper()

	tr := New(sessions)

	mux := http.NewServeMux()
	tr.Register(mux)

	srv := httptest.NewUnstartedServer(mux)
	srv.Config.WriteTimeout = time.Second
	srv.Start()

	t.Cleanup(func() {
		_ = tr.Close()
		srv.Close()
	})

	return tr, srv.URL
}

func open(t *testing.T, url string, header http.Header) *http.Response {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}

	for k, v := range header {
		req.Header[k] = v
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("opening stream: %v", err)
	}

	return resp
}

func connect(t *testing.T, url string, header http.Header) *stream {
	t.Helper()

	resp := open(t, url, header)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	s := &stream{resp: resp, r: bufio.NewReader(resp.Body)}
	t.Cleanup(s.close)

	return s
}

// waitForConns blocks until the transport holds want connections. Registration
// happens on the server goroutine, so it is not observable the instant the
// client's headers arrive.
func waitForConns(t *testing.T, tr *Transport, want int) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)

	for time.Now().Before(deadline) {
		conns, err := tr.Conns()
		if err != nil {
			t.Fatalf("Conns() = %v", err)
		}

		if len(conns) == want {
			return
		}

		time.Sleep(5 * time.Millisecond)
	}

	conns, _ := tr.Conns()
	t.Fatalf("transport holds %d connections, want %d", len(conns), want)
}

func TestStream_RejectsAMissingOrUnknownToken(t *testing.T) {
	sessions := fakeSessions{token: "good-token", id: bson.NewObjectID()}
	_, url := newTestTransport(t, sessions)

	cases := map[string]string{
		"no token":      url + path,
		"unknown token": url + path + "?token=bad-token",
	}

	for name, target := range cases {
		t.Run(name, func(t *testing.T) {
			resp := open(t, target, nil)
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
			}
		})
	}
}

// EventSource cannot set request headers, so the query parameter is the only
// way a browser can authenticate the stream. Native clients may use either.
func TestStream_AuthenticatesByQueryParamOrBearerHeader(t *testing.T) {
	sessions := fakeSessions{token: "good-token", id: bson.NewObjectID()}

	t.Run("query parameter", func(t *testing.T) {
		tr, url := newTestTransport(t, sessions)
		connect(t, url+path+"?token=good-token", nil)
		waitForConns(t, tr, 1)
	})

	t.Run("bearer header", func(t *testing.T) {
		tr, url := newTestTransport(t, sessions)
		connect(t, url+path, http.Header{"Authorization": {"Bearer good-token"}})
		waitForConns(t, tr, 1)
	})
}

func TestStream_SendsEventStreamHeaders(t *testing.T) {
	sessions := fakeSessions{token: "good-token", id: bson.NewObjectID()}
	_, url := newTestTransport(t, sessions)

	s := connect(t, url+path+"?token=good-token", nil)

	want := map[string]string{
		"Content-Type":  "text/event-stream",
		"Cache-Control": "no-cache",
	}

	for header, value := range want {
		if got := s.resp.Header.Get(header); got != value {
			t.Errorf("%s = %q, want %q", header, got, value)
		}
	}
}

func TestStream_DeliversABroadcastAsADataFrame(t *testing.T) {
	sessions := fakeSessions{token: "good-token", id: bson.NewObjectID()}
	tr, url := newTestTransport(t, sessions)

	s := connect(t, url+path+"?token=good-token", nil)
	waitForConns(t, tr, 1)

	if err := tr.Broadcast([]byte(`{"op":"update"}`)); err != nil {
		t.Fatalf("Broadcast() = %v", err)
	}

	if got, want := s.line(t), `data: {"op":"update"}`; got != want {
		t.Errorf("frame = %q, want %q", got, want)
	}
}

// This is the property that makes SSE interchangeable with the other
// transports: the same session-keyed filter reaches it.
func TestStream_IsAddressableByItsSessionKey(t *testing.T) {
	id := bson.NewObjectID()
	sessions := fakeSessions{token: "good-token", id: id}
	tr, url := newTestTransport(t, sessions)

	s := connect(t, url+path+"?token=good-token", nil)
	waitForConns(t, tr, 1)

	match := func(c transport.Conn) bool {
		value, ok := c.Get(transport.SessionIDKey)

		return ok && value == id.Hex()
	}

	if err := tr.BroadcastFilter([]byte(`{"targeted":true}`), match); err != nil {
		t.Fatalf("BroadcastFilter() = %v", err)
	}

	if got, want := s.line(t), `data: {"targeted":true}`; got != want {
		t.Errorf("frame = %q, want %q", got, want)
	}
}

// A multi-line payload must not be split across events: every line needs its
// own "data:" prefix, and a bare newline would end the event early.
func TestStream_EscapesNewlinesInThePayload(t *testing.T) {
	sessions := fakeSessions{token: "good-token", id: bson.NewObjectID()}
	tr, url := newTestTransport(t, sessions)

	s := connect(t, url+path+"?token=good-token", nil)
	waitForConns(t, tr, 1)

	if err := tr.Broadcast([]byte("{\n  \"op\": \"update\"\n}")); err != nil {
		t.Fatalf("Broadcast() = %v", err)
	}

	for _, want := range []string{"data: {", `data:   "op": "update"`, "data: }"} {
		if got := s.line(t); got != want {
			t.Errorf("frame line = %q, want %q", got, want)
		}
	}
}

// Proxies and load balancers drop an idle stream. The heartbeat is a comment
// frame, which clients ignore.
func TestStream_EmitsAHeartbeatWhileIdle(t *testing.T) {
	sessions := fakeSessions{token: "good-token", id: bson.NewObjectID()}
	tr, url := newTestTransport(t, sessions)

	tr.Heartbeat = 20 * time.Millisecond

	s := connect(t, url+path+"?token=good-token", nil)

	if got := s.line(t); !strings.HasPrefix(got, ":") {
		t.Errorf("first idle frame = %q, want a comment frame starting with %q", got, ":")
	}
}

// The stream outlives the server's WriteTimeout, which applies to SSE because
// (unlike a WebSocket upgrade) it never hijacks the connection.
func TestStream_SurvivesTheServerWriteTimeout(t *testing.T) {
	sessions := fakeSessions{token: "good-token", id: bson.NewObjectID()}
	tr, url := newTestTransport(t, sessions)

	tr.Heartbeat = 20 * time.Millisecond

	s := connect(t, url+path+"?token=good-token", nil)

	// newTestTransport sets WriteTimeout to 1s; keep reading past it.
	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		s.line(t)
	}

	waitForConns(t, tr, 1)
}

func TestStream_DisconnectDeregistersTheConnection(t *testing.T) {
	sessions := fakeSessions{token: "good-token", id: bson.NewObjectID()}
	tr, url := newTestTransport(t, sessions)

	s := connect(t, url+path+"?token=good-token", nil)
	waitForConns(t, tr, 1)

	s.close()

	waitForConns(t, tr, 0)
}

// Kicking a player must close their stream, not just stop addressing it.
func TestStream_DisconnectClosesTheStream(t *testing.T) {
	id := bson.NewObjectID()
	sessions := fakeSessions{token: "good-token", id: id}
	tr, url := newTestTransport(t, sessions)

	connect(t, url+path+"?token=good-token", nil)
	waitForConns(t, tr, 1)

	tr.Disconnect([]string{id.Hex()})

	waitForConns(t, tr, 0)
}

// The transport must be usable inside a transport.Multi alongside WebSocket
// and GraphQL.
var _ transport.Transport = (*Transport)(nil)
