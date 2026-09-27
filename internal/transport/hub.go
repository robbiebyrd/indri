package transport

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
)

var (
	// ErrClosed is returned when writing to, or registering with, something
	// that has been closed.
	ErrClosed = errors.New("transport: closed")
	// ErrBufferFull is returned when a connection's outbound queue is full,
	// i.e. the client is not reading fast enough.
	ErrBufferFull = errors.New("transport: outbound buffer full")
)

// Frame is one queued outbound message.
type Frame struct {
	Data   []byte
	Binary bool
}

// QueuedConn is the protocol-independent half of a Conn, shared by transports
// that don't get these guarantees from a library the way ws gets them from
// melody:
//
//   - Writes are queued and drained by a single goroutine the transport owns,
//     because an http.ResponseWriter, a websocket, or a data channel must not
//     be written from the many goroutines that broadcast.
//   - Inbound messages are delivered one at a time, because handlers such as
//     login do check-then-set on connection state.
//   - Disconnect fires exactly once, and only after Connect, because kick
//     closes a conn itself and the transport then reports the same close.
type QueuedConn struct {
	mu   sync.Mutex
	keys map[string]any

	out       chan Frame
	done      chan struct{}
	closeOnce sync.Once
	teardown  func()

	deliverMu sync.Mutex

	lifecycleMu  sync.Mutex
	connected    bool
	disconnected bool
}

// NewQueuedConn returns a conn whose outbound queue holds bufferSize frames.
// teardown, if non-nil, runs once on Close to release the protocol resources
// (e.g. close the socket or peer connection).
func NewQueuedConn(bufferSize int, teardown func()) *QueuedConn {
	return &QueuedConn{
		keys:     make(map[string]any),
		out:      make(chan Frame, bufferSize),
		done:     make(chan struct{}),
		teardown: teardown,
	}
}

func (c *QueuedConn) Get(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	v, ok := c.keys[key]

	return v, ok
}

func (c *QueuedConn) Set(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.keys[key] = value
}

func (c *QueuedConn) UnSet(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.keys, key)
}

func (c *QueuedConn) Write(msg []byte) error {
	return c.enqueue(Frame{Data: msg})
}

func (c *QueuedConn) WriteBinary(msg []byte) error {
	return c.enqueue(Frame{Data: msg, Binary: true})
}

// enqueue never blocks: a slow client gets ErrBufferFull rather than stalling
// the broadcaster. The out channel is never closed, so a send racing Close
// cannot panic.
func (c *QueuedConn) enqueue(f Frame) error {
	if c.IsClosed() {
		return ErrClosed
	}

	select {
	case c.out <- f:
		return nil
	default:
		return ErrBufferFull
	}
}

// Close marks the conn closed synchronously, then runs the teardown once.
func (c *QueuedConn) Close() error {
	c.closeOnce.Do(func() {
		close(c.done)

		if c.teardown != nil {
			c.teardown()
		}
	})

	return nil
}

func (c *QueuedConn) IsClosed() bool {
	select {
	case <-c.done:
		return true
	default:
		return false
	}
}

// Outbound is the queue the transport's single writer drains.
func (c *QueuedConn) Outbound() <-chan Frame { return c.out }

// Done is closed when the conn closes.
func (c *QueuedConn) Done() <-chan struct{} { return c.done }

// Connected fires h.Connect once the client can receive.
func (c *QueuedConn) Connected(h Handlers) {
	c.lifecycleMu.Lock()
	if c.connected || c.disconnected {
		c.lifecycleMu.Unlock()
		return
	}
	c.connected = true
	c.lifecycleMu.Unlock()

	if h.Connect != nil {
		h.Connect(c)
	}
}

// Deliver runs h.Message for msg, serialized per conn.
func (c *QueuedConn) Deliver(h Handlers, msg []byte) {
	if h.Message == nil {
		return
	}

	c.deliverMu.Lock()
	defer c.deliverMu.Unlock()

	h.Message(c, msg)
}

// Disconnected fires h.Disconnect at most once, and only if Connect fired.
func (c *QueuedConn) Disconnected(h Handlers) {
	c.lifecycleMu.Lock()
	fire := c.connected && !c.disconnected
	c.disconnected = true
	c.lifecycleMu.Unlock()

	if fire && h.Disconnect != nil {
		h.Disconnect(c)
	}
}

// Hub is the connection registry shared by the QueuedConn-based transports. It
// implements every Transport method except Register, which is protocol
// specific.
type Hub struct {
	mu       sync.RWMutex
	conns    map[string]*QueuedConn
	closed   bool
	handlers Handlers
}

func NewHub() *Hub {
	return &Hub{conns: make(map[string]*QueuedConn)}
}

func (h *Hub) Handle(handlers Handlers) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.handlers = handlers
}

// Handlers returns the registered lifecycle callbacks.
func (h *Hub) Handlers() Handlers {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return h.handlers
}

// Add registers c under id. It fails once the hub is closed, so a connection
// racing shutdown is refused rather than orphaned.
func (h *Hub) Add(id string, c *QueuedConn) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.closed {
		return ErrClosed
	}

	h.conns[id] = c

	return nil
}

func (h *Hub) Lookup(id string) (*QueuedConn, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	c, ok := h.conns[id]

	return c, ok
}

func (h *Hub) Remove(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	delete(h.conns, id)
}

func (h *Hub) snapshot() []*QueuedConn {
	h.mu.RLock()
	defer h.mu.RUnlock()

	conns := make([]*QueuedConn, 0, len(h.conns))
	for _, c := range h.conns {
		conns = append(conns, c)
	}

	return conns
}

func (h *Hub) Broadcast(msg []byte) error {
	return h.BroadcastFilter(msg, func(Conn) bool { return true })
}

// BroadcastFilter queues msg on every matching conn. A failure on one conn
// (closed, or a slow client's full buffer) goes to the Error handler and does
// not fail the broadcast for the rest.
func (h *Hub) BroadcastFilter(msg []byte, match func(Conn) bool) error {
	if h.IsClosed() {
		return ErrClosed
	}

	handlers := h.Handlers()

	for _, c := range h.snapshot() {
		if !match(c) {
			continue
		}

		if err := c.Write(msg); err != nil && handlers.Error != nil {
			handlers.Error(c, err)
		}
	}

	return nil
}

func (h *Hub) Conns() ([]Conn, error) {
	snapshot := h.snapshot()

	conns := make([]Conn, len(snapshot))
	for i, c := range snapshot {
		conns[i] = c
	}

	return conns, nil
}

// Close refuses new connections and closes every open one.
func (h *Hub) Close() error {
	h.mu.Lock()
	h.closed = true
	h.mu.Unlock()

	for _, c := range h.snapshot() {
		_ = c.Close()
	}

	return nil
}

func (h *Hub) IsClosed() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return h.closed
}

// NewConnectionID returns a 256-bit random hex ID. Holding one lets a client
// act as that connection, so it gets session-token strength and must never be
// logged.
func NewConnectionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	return hex.EncodeToString(b), nil
}
