package sse_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/robbiebyrd/indri/internal/transport"
	"github.com/robbiebyrd/indri/internal/transport/sse"
	"github.com/robbiebyrd/indri/internal/transport/transporttest"
)

func testConfig() sse.Config {
	return sse.Config{
		BufferSize:     256,
		MaxMessageSize: 1024,
		PingPeriod:     time.Minute,
	}
}

// event is one parsed server-sent event.
type event struct {
	name string
	data string
}

// client is a minimal SSE client: it holds the stream open and POSTs sends.
type client struct {
	base   string
	id     string
	cancel context.CancelFunc
	events chan event
}

func dial(t *testing.T, base string) *client {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/sse/stream", nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("opening stream: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		cancel()
		t.Fatalf("stream status %d", resp.StatusCode)
	}

	c := &client{base: base, cancel: cancel, events: make(chan event, 256)}

	go c.read(resp.Body)

	select {
	case ev, ok := <-c.events:
		if !ok || ev.name != "connected" || ev.data == "" {
			t.Fatalf("first event = %+v, want connected with an id", ev)
		}
		c.id = ev.data
	case <-time.After(transporttest.Timeout):
		t.Fatal("no connected event")
	}

	return c
}

// read parses the event stream per the SSE spec, skipping comments.
func (c *client) read(body io.ReadCloser) {
	defer close(c.events)
	defer body.Close()

	sc := bufio.NewScanner(body)

	var (
		name string
		data []string
	)

	for sc.Scan() {
		line := sc.Text()

		switch {
		case line == "":
			if len(data) > 0 {
				c.events <- event{name: name, data: strings.Join(data, "\n")}
			}
			name, data = "", nil
		case strings.HasPrefix(line, ":"):
		case strings.HasPrefix(line, "event: "):
			name = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			data = append(data, strings.TrimPrefix(line, "data: "))
		}
	}
}

func (c *client) post(msg []byte) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, c.base+"/sse/send", bytes.NewReader(msg))
	if err != nil {
		return nil, err
	}

	req.Header.Set(transport.ConnectionIDHeader, c.id)

	return http.DefaultClient.Do(req)
}

func (c *client) Send(msg []byte) error {
	resp, err := c.post(msg)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("send status %d", resp.StatusCode)
	}

	return nil
}

func (c *client) Receive(timeout time.Duration) (transporttest.Frame, error) {
	select {
	case ev, ok := <-c.events:
		if !ok {
			return transporttest.Frame{}, errors.New("stream closed")
		}

		if ev.name == "binary" {
			b, err := base64.StdEncoding.DecodeString(ev.data)
			return transporttest.Frame{Data: b, Binary: true}, err
		}

		return transporttest.Frame{Data: []byte(ev.data)}, nil
	case <-time.After(timeout):
		return transporttest.Frame{}, errors.New("timed out")
	}
}

func (c *client) Close() error {
	c.cancel()
	return nil
}

func TestConformance(t *testing.T) {
	transporttest.Run(t, transporttest.Harness{
		New:  func(*testing.T) transport.Transport { return sse.New(testConfig()) },
		Dial: func(t *testing.T, base string) transporttest.Client { return dial(t, base) },
	})
}

// server starts an SSE transport whose Connect handler records conns.
func server(t *testing.T, cfg sse.Config) (*sse.Transport, string, chan transport.Conn) {
	t.Helper()

	tr := sse.New(cfg)
	connects := make(chan transport.Conn, 8)
	tr.Handle(transport.Handlers{
		Connect: func(c transport.Conn) { connects <- c },
		Message: func(transport.Conn, []byte) {},
	})

	mux := http.NewServeMux()
	tr.Register(mux)
	srv := httptest.NewServer(mux)

	t.Cleanup(func() {
		_ = tr.Close()
		srv.Close()
	})

	return tr, srv.URL, connects
}

func TestStreamHeaders(t *testing.T) {
	_, base, _ := server(t, testConfig())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, base+"/sse/stream", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	want := map[string]string{
		"Content-Type":      "text/event-stream",
		"Cache-Control":     "no-cache",
		"X-Accel-Buffering": "no",
	}
	for k, v := range want {
		if got := resp.Header.Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
}

func TestSend_RejectsUnknownMissingAndStaleIDs(t *testing.T) {
	_, base, connects := server(t, testConfig())

	c := dial(t, base)
	conn := <-connects

	for name, id := range map[string]string{"unknown": strings.Repeat("0", 64), "missing": ""} {
		stranger := &client{base: base, id: id}

		resp, err := stranger.post([]byte("x"))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s id: status %d, want 404", name, resp.StatusCode)
		}
	}

	// After a kick, the old ID must stop working immediately, so it can't be
	// used to act as a connection that no longer exists.
	_ = conn.Close()

	resp, err := c.post([]byte("x"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("stale id: status %d, want 404", resp.StatusCode)
	}
}

func TestSend_RejectsOversizedBody(t *testing.T) {
	cfg := testConfig()
	cfg.MaxMessageSize = 16
	_, base, _ := server(t, cfg)

	c := dial(t, base)

	resp, err := c.post(bytes.Repeat([]byte("x"), 17))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status %d, want 413", resp.StatusCode)
	}
}

func TestRegister_ServesCORSPreflightForSend(t *testing.T) {
	cfg := testConfig()
	cfg.AllowedOrigins = "https://app.example"
	_, base, _ := server(t, cfg)

	req, _ := http.NewRequest(http.MethodOptions, base+"/sse/send", nil)
	req.Header.Set("Origin", "https://app.example")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", transport.ConnectionIDHeader)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent || resp.Header.Get("Access-Control-Allow-Origin") != "https://app.example" {
		t.Fatalf("preflight status %d, allow-origin %q", resp.StatusCode, resp.Header.Get("Access-Control-Allow-Origin"))
	}
}

func TestStream_SendsKeepalivePings(t *testing.T) {
	cfg := testConfig()
	cfg.PingPeriod = 50 * time.Millisecond
	_, base, _ := server(t, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, base+"/sse/stream", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	found := make(chan struct{})
	go func() {
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			if sc.Text() == ": ping" {
				close(found)
				return
			}
		}
	}()

	select {
	case <-found:
	case <-time.After(transporttest.Timeout):
		t.Fatal("no keepalive ping on an idle stream")
	}
}

// TestStream_OutlivesServerTimeouts guards against the production
// http.Server's ReadTimeout/WriteTimeout cutting every stream off. httptest
// servers have no timeouts, so this runs a real one with short timeouts.
func TestStream_OutlivesServerTimeouts(t *testing.T) {
	tr := sse.New(testConfig())
	connects := make(chan transport.Conn, 1)
	tr.Handle(transport.Handlers{Connect: func(c transport.Conn) { connects <- c }})

	mux := http.NewServeMux()
	tr.Register(mux)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 200 * time.Millisecond,
		ReadTimeout:       200 * time.Millisecond,
		WriteTimeout:      200 * time.Millisecond,
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() {
		_ = tr.Close()
		_ = srv.Close()
	})

	c := dial(t, "http://"+ln.Addr().String())
	conn := <-connects

	time.Sleep(600 * time.Millisecond)

	if err := conn.Write([]byte("still here")); err != nil {
		t.Fatalf("write after the server timeouts elapsed: %v", err)
	}

	f, err := c.Receive(transporttest.Timeout)
	if err != nil || string(f.Data) != "still here" {
		t.Fatalf("stream did not survive the server timeouts: %q, %v", f.Data, err)
	}

	if err := c.Send([]byte("upstream too")); err != nil {
		t.Fatalf("send after the server timeouts elapsed: %v", err)
	}
}
