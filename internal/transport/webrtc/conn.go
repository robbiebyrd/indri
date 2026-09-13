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

// fallbackMaxMessageSize is the practical cross-browser DataChannel ceiling,
// not the RTCDataChannel spec's 256 KiB. Firefox and Chromium fragment a
// message above this size incompatibly with each other, so handing one to
// the sink risks it arriving corrupted or not at all -- with nothing wrong at
// this layer to explain why a player's game just stopped updating. Used only
// when the SCTP association's negotiated max-message-size cannot be read
// (newConn); never raise it -- 16 KiB is the cross-browser-safe figure, the
// spec's 256 KiB is not.
const fallbackMaxMessageSize = 16 * 1024

// Chunk framing lets a payload over the ceiling be split into several
// DataChannel messages that the client's ChunkReassembler
// (client/services/webrtc-signal.ts) joins back into one. Every game message
// this transport ever writes is JSON (a models.Game keyframe, an
// events.ChangeEvent delta, ...), and a JSON text's first non-whitespace byte
// is always '{' -- so prefixing a chunk with a NUL byte, which can never
// legally start a JSON document, rules out any ambiguity between a chunk and
// an ordinary message. The receiver tells them apart with one byte read
// instead of parsing the payload to find out. The second byte says whether
// more chunks follow; there is no sequence number, because sendChunked below
// is the only writer and it never interleaves a sequence with anything else
// (see its comment for why that is impossible, not just unlikely).
const (
	chunkMarker       byte = 0x00
	chunkFlagContinue byte = 0x00
	chunkFlagFinal    byte = 0x01
	chunkHeaderSize        = 2
)

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

	// maxMessageSize is the ceiling send chunks against: the SCTP
	// max-message-size negotiated for this peer's association where newConn
	// could read it, else fallbackMaxMessageSize. A message at or under this
	// size goes out verbatim (criterion 4); anything larger is split by
	// sendChunked.
	maxMessageSize uint32

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
//
// negotiatedMaxMessageSize is the SCTP max-message-size pion negotiated for
// this peer's association (criterion 2), read by negotiatedMaxMessageSize in
// webrtc.go -- the same negotiation the browser's own
// RTCSctpTransport.maxMessageSize reports. Zero means "not available" (no
// association yet, or an old/unusual client), and so does any value too
// small to carry even one byte of chunk payload after chunkHeaderSize; both
// fall back to fallbackMaxMessageSize rather than risking a zero or negative
// chunk size in sendChunked.
func newConn(sessionID string, sink dataSink, negotiatedMaxMessageSize uint32) *rtcConn {
	ceiling := negotiatedMaxMessageSize
	if ceiling <= chunkHeaderSize {
		ceiling = fallbackMaxMessageSize
	}

	c := &rtcConn{
		BufferedConn:   transport.NewBufferedConn[[]byte](sessionID, queueSize),
		sink:           sink,
		maxMessageSize: ceiling,
		done:           make(chan struct{}),
		stopped:        make(chan struct{}),
		low:            make(chan struct{}, 1),
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

// send writes msg verbatim if it fits under the ceiling (criterion 4: no
// framing overhead), or splits it into a chunked sequence otherwise.
func (c *rtcConn) send(msg []byte) bool {
	if len(msg) <= int(c.maxMessageSize) {
		return c.writeRaw(msg)
	}

	return c.sendChunked(msg)
}

// writeRaw parks until the sink's BufferedAmount falls back under the
// backpressure threshold, then writes payload verbatim. Waiting on low
// instead of polling BufferedAmount is what keeps this from busy-looping
// while a peer is slow. sendChunked calls this once per chunk, so a chunked
// sequence backs off under a slow peer exactly the same way a single message
// would.
func (c *rtcConn) writeRaw(payload []byte) bool {
	for c.sink.BufferedAmount() >= backpressureThreshold {
		select {
		case <-c.low:
		case <-c.done:
			return false
		}
	}

	if err := c.sink.Send(payload); err != nil {
		log.Printf("webrtc: sending to data channel: %v", err)
	}

	return true
}

// sendChunked splits an over-ceiling payload into chunkMarker-framed pieces
// (see the package-level comment on chunkMarker for the framing format) and
// writes them back-to-back through writeRaw.
//
// drain -- the only goroutine that ever calls send -- does not return to
// select on c.Events() until every chunk here has gone out, so no other
// message, chunked or not, can ever land on the wire between these chunks.
// That is what makes a chunked sequence interleaving with anything else on
// this connection impossible, rather than merely unlikely, and it is why the
// client's reassembler (webrtc-signal.ts's ChunkReassembler) only ever needs
// to track one in-flight sequence per connection.
//
// Chunks carry no sequence number: reassembly relies on the "game"
// DataChannel being reliable and ordered, which is its default -- it is
// created with no RTCDataChannelInit (client/services/webrtc-transport.ts).
// If that channel is ever made unreliable or unordered, chunk reassembly
// breaks.
func (c *rtcConn) sendChunked(msg []byte) bool {
	payloadSize := int(c.maxMessageSize) - chunkHeaderSize

	for offset := 0; offset < len(msg); offset += payloadSize {
		// writeRaw's own backpressure loop only checks done while parked
		// above the BufferedAmount threshold; a sequence sent while the sink
		// is not backed up would otherwise run to completion even after
		// Close, because nothing else in this loop ever looks at done. This
		// check is what stops the NEXT chunk from starting once closed; it
		// cannot interrupt a Send already in flight (see conn_test.go's
		// TestRTCConn_CloseMidChunkSequenceStopsCleanly for why that is the
		// same limitation an unchunked message already has).
		select {
		case <-c.done:
			return false
		default:
		}

		end := offset + payloadSize
		if end > len(msg) {
			end = len(msg)
		}

		flag := chunkFlagContinue
		if end >= len(msg) {
			flag = chunkFlagFinal
		}

		frame := make([]byte, 0, chunkHeaderSize+end-offset)
		frame = append(frame, chunkMarker, flag)
		frame = append(frame, msg[offset:end]...)

		if !c.writeRaw(frame) {
			return false
		}
	}

	return true
}
