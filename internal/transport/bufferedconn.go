package transport

// BufferedConn is a push-only Conn that queues outbound messages on a buffered
// channel for the goroutine that owns the wire to drain: a GraphQL subscription
// resolver returning the channel to gqlgen, or an SSE handler writing frames to
// its response. The element type is parameterised only so a transport can hand
// out the channel type its framework expects.
//
// Build one with NewBufferedConn; the zero value is not usable.
type BufferedConn[T ~[]byte] struct {
	*Keys

	ch chan T
}

// NewBufferedConn returns a connection addressable as sessionID, buffering up
// to size messages.
func NewBufferedConn[T ~[]byte](sessionID string, size int) *BufferedConn[T] {
	return &BufferedConn[T]{
		Keys: NewKeys(sessionID),
		ch:   make(chan T, size),
	}
}

// Events is the channel queued messages arrive on. It is closed by Close.
func (c *BufferedConn[T]) Events() <-chan T { return c.ch }

// Write queues msg. It never blocks and never fails: a client too slow to keep
// up loses messages rather than stalling the broadcaster mid-fan-out, and a
// closed connection silently discards. The client recovers by requesting a
// fresh keyframe, which is what a dropped delta costs.
func (c *BufferedConn[T]) Write(msg []byte) error {
	c.WhileOpen(func() {
		// Copy: the caller reuses its buffer, and the value outlives this call.
		payload := make(T, len(msg))
		copy(payload, msg)

		select {
		case c.ch <- payload:
		default:
		}
	})

	return nil
}

// Close releases the channel exactly once, however many times it is called.
func (c *BufferedConn[T]) Close() error {
	if c.MarkClosed() {
		close(c.ch)
	}

	return nil
}
