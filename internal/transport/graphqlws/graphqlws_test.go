package graphqlws_test

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/robbiebyrd/indri/internal/transport"
	"github.com/robbiebyrd/indri/internal/transport/graphqlws"
	"github.com/robbiebyrd/indri/internal/transport/transporttest"
)

const eventsID = "events"

func testConfig() graphqlws.Config {
	return graphqlws.Config{
		BufferSize:     256,
		MaxMessageSize: 4096,
		PingPeriod:     time.Minute,
		PongWait:       2 * time.Minute,
		InitTimeout:    transporttest.Timeout,
	}
}

type message struct {
	ID      string          `json:"id,omitempty"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type client struct {
	ws   *websocket.Conn
	sent atomic.Int32
}

func rawDial(t *testing.T, base string, subprotocols ...string) *websocket.Conn {
	t.Helper()

	d := websocket.Dialer{Subprotocols: subprotocols}

	ws, _, err := d.Dial("ws"+strings.TrimPrefix(base, "http")+"/graphql", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	t.Cleanup(func() { _ = ws.Close() })

	return ws
}

func write(t *testing.T, ws *websocket.Conn, m any) {
	t.Helper()

	if err := ws.WriteJSON(m); err != nil {
		t.Fatalf("write %+v: %v", m, err)
	}
}

func read(t *testing.T, ws *websocket.Conn) message {
	t.Helper()

	_ = ws.SetReadDeadline(time.Now().Add(transporttest.Timeout))

	var m message
	if err := ws.ReadJSON(&m); err != nil {
		t.Fatalf("read: %v", err)
	}

	return m
}

// closeCode reads until the server closes the socket and returns the code.
func closeCode(t *testing.T, ws *websocket.Conn) int {
	t.Helper()

	_ = ws.SetReadDeadline(time.Now().Add(transporttest.Timeout))

	for {
		if _, _, err := ws.ReadMessage(); err != nil {
			var ce *websocket.CloseError
			if errors.As(err, &ce) {
				return ce.Code
			}

			t.Fatalf("socket ended without a close frame: %v", err)
		}
	}
}

func handshake(t *testing.T, ws *websocket.Conn) {
	t.Helper()

	write(t, ws, message{Type: "connection_init"})

	if m := read(t, ws); m.Type != "connection_ack" {
		t.Fatalf("got %+v, want connection_ack", m)
	}
}

func subscribeEvents(t *testing.T, ws *websocket.Conn, id string) {
	t.Helper()

	write(t, ws, map[string]any{
		"id":   id,
		"type": "subscribe",
		"payload": map[string]any{
			"operationName": "IndriEvents",
			"query":         "subscription IndriEvents { indriEvents }",
		},
	})
}

func sendOp(id string, msg string) map[string]any {
	return map[string]any{
		"id":   id,
		"type": "subscribe",
		"payload": map[string]any{
			"operationName": "Send",
			"query":         "mutation Send($message: String!) { send(message: $message) }",
			"variables":     map[string]any{"message": msg},
		},
	}
}

func dial(t *testing.T, base string) transporttest.Client {
	ws := rawDial(t, base, "graphql-transport-ws")
	handshake(t, ws)
	subscribeEvents(t, ws, eventsID)

	return &client{ws: ws}
}

func (c *client) Send(msg []byte) error {
	n := c.sent.Add(1)
	return c.ws.WriteJSON(sendOp(fmt.Sprintf("send-%d", n), string(msg)))
}

func (c *client) Receive(timeout time.Duration) (transporttest.Frame, error) {
	_ = c.ws.SetReadDeadline(time.Now().Add(timeout))

	for {
		var m message
		if err := c.ws.ReadJSON(&m); err != nil {
			return transporttest.Frame{}, err
		}

		switch {
		case m.Type == "next" && m.ID == eventsID:
			var result struct {
				Data struct {
					IndriEvents struct {
						Text *string `json:"text"`
						B64  *string `json:"b64"`
					} `json:"indriEvents"`
				} `json:"data"`
			}
			if err := json.Unmarshal(m.Payload, &result); err != nil {
				return transporttest.Frame{}, err
			}

			ev := result.Data.IndriEvents
			if ev.B64 != nil {
				b, err := base64.StdEncoding.DecodeString(*ev.B64)
				return transporttest.Frame{Data: b, Binary: true}, err
			}
			if ev.Text != nil {
				return transporttest.Frame{Data: []byte(*ev.Text)}, nil
			}

			return transporttest.Frame{}, fmt.Errorf("next without text or b64: %s", m.Payload)
		case m.Type == "complete" && m.ID == eventsID:
			return transporttest.Frame{}, errors.New("events subscription completed")
		case m.Type == "complete":
			// A Send finished; not a frame.
		default:
			return transporttest.Frame{}, fmt.Errorf("unexpected message %+v", m)
		}
	}
}

func (c *client) Close() error { return c.ws.Close() }

func TestConformance(t *testing.T) {
	transporttest.Run(t, transporttest.Harness{
		New:  func(*testing.T) transport.Transport { return graphqlws.New(testConfig()) },
		Dial: dial,
	})
}

type recorder struct {
	connects, disconnects, messages atomic.Int32
}

func server(t *testing.T, cfg graphqlws.Config) (string, *recorder) {
	t.Helper()

	rec := &recorder{}
	tr := graphqlws.New(cfg)
	tr.Handle(transport.Handlers{
		Connect:    func(transport.Conn) { rec.connects.Add(1) },
		Disconnect: func(transport.Conn) { rec.disconnects.Add(1) },
		Message:    func(transport.Conn, []byte) { rec.messages.Add(1) },
	})

	mux := http.NewServeMux()
	tr.Register(mux)
	srv := httptest.NewServer(mux)

	t.Cleanup(func() {
		_ = tr.Close()
		srv.Close()
	})

	return srv.URL, rec
}

func TestProtocolViolationsCloseWithSpecCodes(t *testing.T) {
	cases := []struct {
		name string
		cfg  func(*graphqlws.Config)
		run  func(t *testing.T, ws *websocket.Conn)
		want int
		sub  []string
	}{
		{
			name: "subprotocol not offered",
			sub:  []string{},
			run:  func(*testing.T, *websocket.Conn) {},
			want: 4406,
		},
		{
			name: "no connection_init in time",
			cfg:  func(c *graphqlws.Config) { c.InitTimeout = 50 * time.Millisecond },
			run:  func(*testing.T, *websocket.Conn) {},
			want: 4408,
		},
		{
			name: "duplicate connection_init",
			run: func(t *testing.T, ws *websocket.Conn) {
				handshake(t, ws)
				write(t, ws, message{Type: "connection_init"})
			},
			want: 4429,
		},
		{
			name: "subscribe before connection_ack",
			run:  func(t *testing.T, ws *websocket.Conn) { subscribeEvents(t, ws, "x") },
			want: 4401,
		},
		{
			name: "subscription id already in use",
			run: func(t *testing.T, ws *websocket.Conn) {
				handshake(t, ws)
				subscribeEvents(t, ws, eventsID)
				write(t, ws, sendOp(eventsID, "{}"))
			},
			want: 4409,
		},
		{
			name: "malformed message",
			run: func(t *testing.T, ws *websocket.Conn) {
				if err := ws.WriteMessage(websocket.TextMessage, []byte("not json")); err != nil {
					t.Fatal(err)
				}
			},
			want: 4400,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := testConfig()
			if tc.cfg != nil {
				tc.cfg(&cfg)
			}

			base, _ := server(t, cfg)

			sub := tc.sub
			if sub == nil {
				sub = []string{"graphql-transport-ws"}
			}

			ws := rawDial(t, base, sub...)
			tc.run(t, ws)

			if got := closeCode(t, ws); got != tc.want {
				t.Fatalf("close code %d, want %d", got, tc.want)
			}
		})
	}
}

func TestPingGetsPong(t *testing.T) {
	base, _ := server(t, testConfig())
	ws := rawDial(t, base, "graphql-transport-ws")
	handshake(t, ws)

	write(t, ws, message{Type: "ping"})

	if m := read(t, ws); m.Type != "pong" {
		t.Fatalf("got %+v, want pong", m)
	}
}

func TestSendBeforeEventsSubscriptionIsAnOperationError(t *testing.T) {
	base, rec := server(t, testConfig())
	ws := rawDial(t, base, "graphql-transport-ws")
	handshake(t, ws)

	write(t, ws, sendOp("early", "{}"))

	m := read(t, ws)
	if m.Type != "error" || m.ID != "early" {
		t.Fatalf("got %+v, want error for id early", m)
	}
	if rec.messages.Load() != 0 {
		t.Fatal("message delivered before Connect")
	}
}

func TestUnknownOperationIsAnOperationError(t *testing.T) {
	base, _ := server(t, testConfig())
	ws := rawDial(t, base, "graphql-transport-ws")
	handshake(t, ws)

	write(t, ws, map[string]any{
		"id": "q", "type": "subscribe",
		"payload": map[string]any{"operationName": "Other", "query": "{ other }"},
	})

	if m := read(t, ws); m.Type != "error" || m.ID != "q" {
		t.Fatalf("got %+v, want error for id q", m)
	}
}

func TestSendCompletesItsOperation(t *testing.T) {
	base, rec := server(t, testConfig())
	ws := rawDial(t, base, "graphql-transport-ws")
	handshake(t, ws)
	subscribeEvents(t, ws, eventsID)

	write(t, ws, sendOp("s1", `{"action":"refresh"}`))

	if m := read(t, ws); m.Type != "complete" || m.ID != "s1" {
		t.Fatalf("got %+v, want complete for s1", m)
	}
	if rec.messages.Load() != 1 {
		t.Fatalf("messages = %d, want 1", rec.messages.Load())
	}
}

func TestClientCompletingEventsDisconnects(t *testing.T) {
	base, rec := server(t, testConfig())
	ws := rawDial(t, base, "graphql-transport-ws")
	handshake(t, ws)
	subscribeEvents(t, ws, eventsID)

	deadline := time.Now().Add(transporttest.Timeout)
	for rec.connects.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}

	write(t, ws, message{ID: eventsID, Type: "complete"})

	for rec.disconnects.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if rec.disconnects.Load() != 1 {
		t.Fatalf("disconnects = %d, want 1", rec.disconnects.Load())
	}
}
