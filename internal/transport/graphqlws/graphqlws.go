// Package graphqlws implements transport.Transport over one WebSocket
// speaking the graphql-transport-ws subprotocol. It frames Indri's messages
// as GraphQL operations; it does not execute GraphQL, because Indri's actions
// are registered dynamically and have no static schema.
//
// Wire format, after connection_init / connection_ack:
//
//	subscribe {operationName: "IndriEvents"}   opens the server-to-client
//	    stream. Each message arrives as a next frame whose payload is
//	    {"data":{"indriEvents":{"text":"…"}}} or, for binary,
//	    {"data":{"indriEvents":{"b64":"…"}}}. Completing it disconnects.
//	subscribe {operationName: "Send", variables: {message: "…"} | {b64: "…"}}
//	    delivers one client-to-server message; the server replies complete.
//
// Protocol violations close the socket with the subprotocol's codes: 4400
// invalid message, 4401 subscribe before ack, 4406 subprotocol not offered,
// 4408 no connection_init in time, 4409 subscription id in use, 4429
// duplicate connection_init.
package graphqlws

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"github.com/robbiebyrd/indri/internal/transport"
)

const (
	path        = "/graphql"
	subprotocol = "graphql-transport-ws"

	opEvents = "IndriEvents"
	opSend   = "Send"
)

// Config tunes the transport.
type Config struct {
	// AllowedOrigins is the comma-separated browser Origin allowlist.
	AllowedOrigins string
	// BufferSize is how many outbound messages may queue per connection.
	BufferSize int
	// MaxMessageSize caps one incoming WebSocket message, in bytes.
	MaxMessageSize int64
	// PingPeriod is how often the server pings to detect dead clients.
	PingPeriod time.Duration
	// PongWait is how long the server waits for any read before giving up.
	PongWait time.Duration
	// InitTimeout is how long a client has to send connection_init.
	InitTimeout time.Duration
}

// Transport is a graphql-transport-ws transport.
type Transport struct {
	*transport.Hub
	cfg      Config
	upgrader websocket.Upgrader
}

func New(cfg Config) *Transport {
	return &Transport{
		Hub: transport.NewHub(),
		cfg: cfg,
		upgrader: websocket.Upgrader{
			Subprotocols: []string{subprotocol},
			CheckOrigin:  transport.OriginChecker(cfg.AllowedOrigins),
		},
	}
}

func (t *Transport) Register(mux *http.ServeMux) {
	mux.HandleFunc(http.MethodGet+" "+path, t.serve)
}

type message struct {
	ID      string          `json:"id,omitempty"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type operation struct {
	OperationName string `json:"operationName"`
	Variables     struct {
		Message *string `json:"message"`
		B64     *string `json:"b64"`
	} `json:"variables"`
}

// socket is one WebSocket connection. Only its writer goroutine calls the
// gorilla write methods; closing uses WriteControl, which gorilla allows
// concurrently with everything else.
type socket struct {
	t    *Transport
	ws   *websocket.Conn
	ctrl chan message

	attach     chan *transport.QueuedConn
	stop       chan struct{}
	writerDone chan struct{}

	acked    atomic.Bool
	closing  sync.Once
	eventsID string
	conn     *transport.QueuedConn
}

func (t *Transport) serve(w http.ResponseWriter, r *http.Request) {
	ws, err := t.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return // Upgrade already wrote the HTTP error.
	}

	s := &socket{
		t:          t,
		ws:         ws,
		ctrl:       make(chan message, 16),
		attach:     make(chan *transport.QueuedConn, 1),
		stop:       make(chan struct{}),
		writerDone: make(chan struct{}),
	}

	defer s.shutdown()

	if ws.Subprotocol() != subprotocol {
		s.closeWith(4406, "Subprotocol not acceptable")
		return
	}

	ws.SetReadLimit(t.cfg.MaxMessageSize)
	s.extendReadDeadline()
	ws.SetPongHandler(func(string) error { s.extendReadDeadline(); return nil })

	init := time.AfterFunc(t.cfg.InitTimeout, func() {
		if !s.acked.Load() {
			s.closeWith(4408, "Connection initialisation timeout")
		}
	})
	defer init.Stop()

	go s.writer()

	s.read()
}

func (s *socket) extendReadDeadline() {
	_ = s.ws.SetReadDeadline(time.Now().Add(s.t.cfg.PongWait))
}

// read handles messages one at a time, which is what keeps a client's
// messages in order.
func (s *socket) read() {
	for {
		_, data, err := s.ws.ReadMessage()
		if err != nil {
			return
		}

		s.extendReadDeadline()

		var m message
		if err := json.Unmarshal(data, &m); err != nil || m.Type == "" {
			s.closeWith(4400, "Invalid message received")
			return
		}

		if !s.handle(m) {
			return
		}
	}
}

// handle processes one protocol message and reports whether to keep reading.
func (s *socket) handle(m message) bool {
	switch m.Type {
	case "connection_init":
		if s.acked.Swap(true) {
			s.closeWith(4429, "Too many initialisation requests")
			return false
		}

		s.reply(message{Type: "connection_ack"})
	case "ping":
		s.reply(message{Type: "pong"})
	case "pong":
	case "subscribe":
		return s.subscribe(m)
	case "complete":
		if s.conn != nil && m.ID == s.eventsID {
			_ = s.conn.Close()
			return false
		}
	default:
		s.closeWith(4400, "Invalid message received")
		return false
	}

	return true
}

func (s *socket) subscribe(m message) bool {
	if !s.acked.Load() {
		s.closeWith(4401, "Unauthorized")
		return false
	}

	if m.ID == "" {
		s.closeWith(4400, "Invalid message received")
		return false
	}

	if s.conn != nil && m.ID == s.eventsID {
		s.closeWith(4409, fmt.Sprintf("Subscriber for %s already exists", m.ID))
		return false
	}

	var op operation
	if err := json.Unmarshal(m.Payload, &op); err != nil {
		s.closeWith(4400, "Invalid message received")
		return false
	}

	switch op.OperationName {
	case opEvents:
		if s.conn != nil {
			s.operationError(m.ID, "already subscribed to "+opEvents)
			return true
		}

		return s.openEvents(m.ID)
	case opSend:
		if s.conn == nil {
			s.operationError(m.ID, "subscribe to "+opEvents+" before sending")
			return true
		}

		msg, err := op.message()
		if err != nil {
			s.operationError(m.ID, err.Error())
			return true
		}

		s.conn.Deliver(s.t.Handlers(), msg)
		s.reply(message{ID: m.ID, Type: "complete"})
	default:
		s.operationError(m.ID, fmt.Sprintf("unknown operation %q", op.OperationName))
	}

	return true
}

// openEvents turns the socket into an Indri connection. Connect fires only
// now, because before this subscription there is no id to deliver on.
func (s *socket) openEvents(id string) bool {
	connID, err := transport.NewConnectionID()
	if err != nil {
		s.closeWith(websocket.CloseInternalServerErr, "could not open connection")
		return false
	}

	conn := transport.NewQueuedConn(s.t.cfg.BufferSize, nil)
	if err := s.t.Hub.Add(connID, conn); err != nil {
		s.closeWith(websocket.CloseGoingAway, "server shutting down")
		return false
	}

	s.eventsID = id
	s.conn = conn
	s.attach <- conn

	handlers := s.t.Handlers()
	go func() {
		<-s.stop
		s.t.Hub.Remove(connID)
		_ = conn.Close()
		conn.Disconnected(handlers)
	}()

	conn.Connected(handlers)

	return true
}

func (op operation) message() ([]byte, error) {
	switch {
	case op.Variables.Message != nil:
		return []byte(*op.Variables.Message), nil
	case op.Variables.B64 != nil:
		return base64.StdEncoding.DecodeString(*op.Variables.B64)
	default:
		return nil, fmt.Errorf("%s needs a message or b64 variable", opSend)
	}
}

// reply queues a protocol message for the writer. If the writer has already
// exited (the socket is dead), the message is dropped rather than blocking
// the read loop forever.
func (s *socket) reply(m message) {
	select {
	case s.ctrl <- m:
	case <-s.writerDone:
	}
}

func (s *socket) operationError(id, reason string) {
	payload, _ := json.Marshal([]map[string]string{{"message": reason}})
	s.reply(message{ID: id, Type: "error", Payload: payload})
}

// writer is the only goroutine that writes data frames to the socket.
func (s *socket) writer() {
	defer close(s.writerDone)

	ping := time.NewTicker(s.t.cfg.PingPeriod)
	defer ping.Stop()

	var (
		conn *transport.QueuedConn
		out  <-chan transport.Frame
		done <-chan struct{}
	)

	for {
		select {
		case m := <-s.ctrl:
			if s.ws.WriteJSON(m) != nil {
				return
			}
		case conn = <-s.attach:
			out, done = conn.Outbound(), conn.Done()
		case f := <-out:
			if !s.writeFrame(f) {
				return
			}
		case <-done:
			// A server-side close (kick, shutdown): flush what was queued,
			// end the subscription, then close the socket.
			flushed := true
			conn.Drain(func(f transport.Frame) bool {
				flushed = s.writeFrame(f)
				return flushed
			})

			if flushed {
				_ = s.ws.WriteJSON(message{ID: s.eventsID, Type: "complete"})
			}

			s.closeWith(websocket.CloseNormalClosure, "")

			return
		case <-ping.C:
			if s.ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(time.Second)) != nil {
				return
			}
		case <-s.stop:
			return
		}
	}
}

func (s *socket) writeFrame(f transport.Frame) bool {
	event := map[string]string{"text": string(f.Data)}
	if f.Binary {
		event = map[string]string{"b64": base64.StdEncoding.EncodeToString(f.Data)}
	}

	payload, err := json.Marshal(map[string]any{"data": map[string]any{"indriEvents": event}})
	if err != nil {
		log.Printf("graphqlws: encoding frame: %v", err)
		return true
	}

	return s.ws.WriteJSON(message{ID: s.eventsID, Type: "next", Payload: payload}) == nil
}

// closeWith sends a close frame with code and closes the socket, which ends
// the read loop. Safe from any goroutine.
func (s *socket) closeWith(code int, reason string) {
	s.closing.Do(func() {
		msg := websocket.FormatCloseMessage(code, reason)
		_ = s.ws.WriteControl(websocket.CloseMessage, msg, time.Now().Add(time.Second))
		_ = s.ws.Close()
	})
}

// shutdown runs when the read loop ends: it stops the writer and fires the
// connection's Disconnect.
func (s *socket) shutdown() {
	close(s.stop)
	s.closeWith(websocket.CloseNormalClosure, "")
}
