package webrtc

import (
	"log"
	"sync"

	"github.com/robbiebyrd/indri/internal/transport"
)

// queueSize matches sse.streamBuffer: a client too far behind drops its oldest
// queued deltas and recovers with a refresh, the same contract as every other
// push transport.
const queueSize = 16

// backpressureThreshold is the BufferedAmount high-water mark. The drain
// goroutine parks once the sink reports at or above it, and resumes once
// OnBufferedAmountLow reports it has dropped back below the same value passed
// to SetBufferedAmountLowThreshold.
const backpressureThreshold = 1 << 20 // 1 MiB

// maxMessageSize is the practical cross-browser DataChannel ceiling, not the
// RTCDataChannel spec's 256 KiB. Firefox and Chromium fragment a message
// above this size incompatibly with each other, so handing one to the sink
// risks it arriving corrupted or not at all -- with nothing wrong at this
// layer to explain why a player's game just stopped updating. A keyframe can
// exceed it; chunking is deferred (plan Open Question 3), so for now an
// oversized payload is refused outright rather than truncated or dropped
// with no trace.
const maxMessageSize = 16 * 1024

// dataSink is the half of pion's DataChannel this conn uses, so tests need no
// real PeerConnection.
type dataSink interface {
	Send([]byte) error
	BufferedAmount() uint64
	OnBufferedAmountLow(func())
	SetBufferedAmountLowThreshold(uint64)
}

// rtcConn is a transport.Conn backed by a WebRTC DataChannel. pion documents
// no goroutine-safety guarantee for DataChannel.Send, so every write is
// serialised through the drain goroutine started in newConn; nothing else may
// call sink.Send.
type rtcConn struct {
	*transport.BufferedConn[[]byte]

	sink dataSink

	// done stops the drain goroutine, including while it is parked waiting on
	// low. closeOnce guards it so a double Close cannot double-close it.
	done      chan struct{}
	closeOnce sync.Once

	// stopped is closed when the drain goroutine returns, so Close can be
	// proven to actually stop it rather than assumed.
	stopped chan struct{}

	// low is signalled by the OnBufferedAmountLow callback. Buffered by one so
	// a callback firing while the drain isn't yet waiting is not lost.
	low chan struct{}

	// onTeardown, when set, is invoked by Close (never by closeConn) once
	// this conn's own plumbing has stopped. attachGame wires it to the owning
	// peer's teardown, so closing this conn from anywhere -- including a kick
	// reaching it through Registry.Disconnect -- also releases the
	// PeerConnection, not just this conn: without it, the ICE agent, DTLS and
	// SCTP resources survive until pion's own Failed callback fires. peer.close
	// calls closeConn instead of Close for exactly this reason: it already
	// runs inside that same teardown (guarded by peer.teardown, a sync.Once),
	// and looping back into onTeardown from there would call Once.Do
	// reentrantly on the same goroutine -- documented by sync.Once as a
	// deadlock, not something the Once guards against.
	onTeardown func()
}

var _ transport.Conn = (*rtcConn)(nil)

// newConn wraps sink in a transport.Conn and starts the goroutine that owns
// every write to it. An empty sessionID matches transport.NewKeys: a peer
// that has not logged in yet carries no session key.
func newConn(sessionID string, sink dataSink) *rtcConn {
	c := &rtcConn{
		BufferedConn: transport.NewBufferedConn[[]byte](sessionID, queueSize),
		sink:         sink,
		done:         make(chan struct{}),
		stopped:      make(chan struct{}),
		low:          make(chan struct{}, 1),
	}

	sink.SetBufferedAmountLowThreshold(backpressureThreshold)
	sink.OnBufferedAmountLow(func() {
		select {
		case c.low <- struct{}{}:
		default:
		}
	})

	go c.drain()

	return c
}

// Close stops the drain goroutine, releases the queue, and (if set) tears
// down the owning peer -- however many times Close itself is called. Use
// closeConn instead from inside peer.close; see onTeardown's comment.
func (c *rtcConn) Close() error {
	err := c.closeConn()

	if c.onTeardown != nil {
		c.onTeardown()
	}

	return err
}

// closeConn is Close's actual plumbing-teardown, split out so peer.close can
// invoke it without also firing onTeardown -- see onTeardown's comment for
// why looping back through Close there would deadlock. BufferedConn.Close
// guards the queue's channel; closeOnce guards done separately, because a
// drain parked on low (not reading Events) would otherwise never see the
// queue close.
func (c *rtcConn) closeConn() error {
	err := c.BufferedConn.Close()

	c.closeOnce.Do(func() { close(c.done) })

	return err
}

// drain owns every write to the sink: pion documents no goroutine-safety
// guarantee for Send, and one owning goroutine also gives us a place to
// honour BufferedAmount so a slow peer backs up in its own queue, not in
// SCTP.
func (c *rtcConn) drain() {
	defer close(c.stopped)

	for {
		select {
		case <-c.done:
			return
		case msg, open := <-c.Events():
			if !open {
				return
			}

			if !c.send(msg) {
				return
			}
		}
	}
}

// send parks until the sink's BufferedAmount falls back under the
// backpressure threshold, then writes msg. Waiting on low instead of polling
// BufferedAmount is what keeps this from busy-looping while a peer is slow.
func (c *rtcConn) send(msg []byte) bool {
	if len(msg) > maxMessageSize {
		// Report and log, then move on to the next queued message: dropping
		// silently here would mean a player's client simply stops receiving
		// updates with nothing anywhere to say why.
		log.Printf(
			"webrtc: dropping %d byte payload, exceeds the %d byte cross-browser DataChannel ceiling",
			len(msg), maxMessageSize,
		)

		return true
	}

	for c.sink.BufferedAmount() >= backpressureThreshold {
		select {
		case <-c.low:
		case <-c.done:
			return false
		}
	}

	if err := c.sink.Send(msg); err != nil {
		log.Printf("webrtc: sending to data channel: %v", err)
	}

	return true
}
