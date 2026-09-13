package webrtc

import (
	"bytes"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/robbiebyrd/indri/internal/transport"
)

// fakeSink is the dataSink test double. It never touches pion: every test in
// this file drives it directly, proving the conn needs no real PeerConnection.
type fakeSink struct {
	mu sync.Mutex

	sent           [][]byte
	bufferedAmount uint64
	lowCallback    func()
	sendErr        error

	// sendBlock, when non-nil, blocks Send until closed. It simulates a slow
	// peer whose Send call itself hangs, so writes pile up in the conn's queue
	// instead of being drained.
	sendBlock chan struct{}
	// sendStarted is closed the first time Send is entered, so a test can wait
	// for a blocking Send to actually be in flight before it starts asserting
	// queue behaviour.
	sendStarted     chan struct{}
	sendStartedOnce sync.Once

	sendCalls           int32
	bufferedAmountCalls int32
}

func newFakeSink() *fakeSink {
	return &fakeSink{
		sendStarted: make(chan struct{}),
	}
}

func (f *fakeSink) Send(b []byte) error {
	f.sendStartedOnce.Do(func() { close(f.sendStarted) })

	f.mu.Lock()
	block := f.sendBlock
	f.mu.Unlock()

	if block != nil {
		<-block
	}

	atomic.AddInt32(&f.sendCalls, 1)

	cp := make([]byte, len(b))
	copy(cp, b)

	f.mu.Lock()
	f.sent = append(f.sent, cp)
	err := f.sendErr
	f.mu.Unlock()

	return err
}

func (f *fakeSink) BufferedAmount() uint64 {
	atomic.AddInt32(&f.bufferedAmountCalls, 1)

	f.mu.Lock()
	defer f.mu.Unlock()

	return f.bufferedAmount
}

func (f *fakeSink) OnBufferedAmountLow(cb func()) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.lowCallback = cb
}

func (f *fakeSink) SetBufferedAmountLowThreshold(uint64) {}

func (f *fakeSink) setBufferedAmount(v uint64) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.bufferedAmount = v
}

func (f *fakeSink) setSendBlock(ch chan struct{}) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.sendBlock = ch
}

// fireLow invokes the callback the conn registered via OnBufferedAmountLow,
// exactly like pion would after BufferedAmount drops below the low threshold.
func (f *fakeSink) fireLow() {
	f.mu.Lock()
	cb := f.lowCallback
	f.mu.Unlock()

	if cb == nil {
		panic("fireLow called before the conn registered a callback")
	}

	cb()
}

func (f *fakeSink) sentMessages() [][]byte {
	f.mu.Lock()
	defer f.mu.Unlock()

	out := make([][]byte, len(f.sent))
	copy(out, f.sent)

	return out
}

func (f *fakeSink) sendCallCount() int32 {
	return atomic.LoadInt32(&f.sendCalls)
}

func (f *fakeSink) bufferedAmountCallCount() int32 {
	return atomic.LoadInt32(&f.bufferedAmountCalls)
}

// waitForSentCount polls until the sink has received n messages or the
// deadline expires, so tests don't race the drain goroutine.
func waitForSentCount(t *testing.T, sink *fakeSink, n int) {
	t.Helper()

	deadline := time.After(time.Second)
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()

	for {
		if len(sink.sentMessages()) >= n {
			return
		}

		select {
		case <-tick.C:
		case <-deadline:
			t.Fatalf("timed out waiting for the sink to receive %d messages, got %d", n, len(sink.sentMessages()))
		}
	}
}

// Criterion 1: the conn embeds transport.BufferedConn so it inherits the
// shared drop-oldest queue. This only compiles if the embedding is real.
func TestRTCConn_EmbedsBufferedConn(t *testing.T) {
	conn := newConn("session-1", newFakeSink(), 0)
	defer conn.Close()

	var _ *transport.BufferedConn[[]byte] = conn.BufferedConn

	var _ transport.Conn = conn
}

// Criterion 2: writes reach the sink in order.
func TestRTCConn_WritesReachSinkInOrder(t *testing.T) {
	sink := newFakeSink()
	conn := newConn("session-1", sink, 0)
	defer conn.Close()

	want := [][]byte{[]byte("one"), []byte("two"), []byte("three")}
	for _, msg := range want {
		if err := conn.Write(msg); err != nil {
			t.Fatalf("Write(%q) returned error: %v", msg, err)
		}
	}

	waitForSentCount(t, sink, len(want))

	got := sink.sentMessages()
	if len(got) != len(want) {
		t.Fatalf("sink received %d messages, want %d", len(got), len(want))
	}

	for i, msg := range want {
		if string(got[i]) != string(msg) {
			t.Errorf("message %d = %q, want %q (order not preserved)", i, got[i], msg)
		}
	}
}

// Criterion 3: the seventeenth queued message is dropped rather than blocking
// the broadcaster. The sink's Send is blocked so nothing drains the queue,
// which is the only way to observe the queue actually filling to capacity.
func TestRTCConn_SeventeenthMessageDropped(t *testing.T) {
	sink := newFakeSink()
	block := make(chan struct{})
	sink.setSendBlock(block)

	var unblockOnce sync.Once
	unblock := func() { unblockOnce.Do(func() { close(block) }) }

	conn := newConn("session-1", sink, 0)
	defer func() {
		unblock()
		conn.Close()
	}()

	// warmup is picked up by the drain goroutine immediately and blocks
	// inside Send, so the queue behind it starts empty.
	if err := conn.Write([]byte("warmup")); err != nil {
		t.Fatalf("Write(warmup) returned error: %v", err)
	}

	select {
	case <-sink.sendStarted:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the drain goroutine to start sending warmup")
	}

	// The queue holds 16. Write 17 more; the broadcaster must never block.
	const attempts = 17

	done := make(chan struct{})

	go func() {
		for i := 0; i < attempts; i++ {
			if err := conn.Write([]byte{byte(i)}); err != nil {
				t.Errorf("Write attempt %d returned error: %v", i, err)
			}
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Write blocked instead of returning promptly; the broadcaster must never stall on a slow peer")
	}

	unblock()

	// warmup + 16 of the 17 attempts should reach the sink; the 17th attempt
	// must have been dropped rather than delivered or left the caller blocked.
	waitForSentCount(t, sink, 1+16)

	// Give the drain goroutine a moment to see if it (incorrectly) sends more
	// than 16 of the 17 attempted messages.
	time.Sleep(50 * time.Millisecond)

	got := sink.sentMessages()
	if len(got) != 1+16 {
		t.Fatalf("sink received %d messages, want %d (warmup + 16 of the 17 attempts)", len(got), 1+16)
	}
}

// Criterion 4: Close stops the drain goroutine, proven with a done channel
// rather than assumed.
func TestRTCConn_CloseStopsDrainGoroutine(t *testing.T) {
	conn := newConn("session-1", newFakeSink(), 0)

	select {
	case <-conn.stopped:
		t.Fatal("drain goroutine reported stopped before Close was ever called")
	default:
	}

	if err := conn.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	select {
	case <-conn.stopped:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the drain goroutine to stop after Close")
	}
}

// Criterion 5: calling Close twice is safe.
func TestRTCConn_DoubleCloseIsSafe(t *testing.T) {
	conn := newConn("session-1", newFakeSink(), 0)

	if err := conn.Close(); err != nil {
		t.Fatalf("first Close returned error: %v", err)
	}

	if err := conn.Close(); err != nil {
		t.Fatalf("second Close returned error: %v", err)
	}
}

// Criterion 6: a sink reporting high BufferedAmount parks the drain instead
// of continuing to send, and it parks by waiting on OnBufferedAmountLow
// rather than busy-looping on BufferedAmount.
func TestRTCConn_HighBufferedAmountParksDrain(t *testing.T) {
	sink := newFakeSink()
	sink.setBufferedAmount(backpressureThreshold + 1)

	conn := newConn("session-1", sink, 0)
	defer conn.Close()

	if err := conn.Write([]byte("blocked")); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	// Give the drain goroutine time to act. It must park, not send.
	time.Sleep(50 * time.Millisecond)

	if calls := sink.sendCallCount(); calls != 0 {
		t.Fatalf("Send called %d times while BufferedAmount stayed high; drain must park instead of sending", calls)
	}

	// A busy loop would call BufferedAmount() far more than once while
	// parked; parking on the callback means exactly one check before it
	// blocks on the low-water signal.
	if calls := sink.bufferedAmountCallCount(); calls != 1 {
		t.Fatalf("BufferedAmount polled %d times while parked; drain must wait on OnBufferedAmountLow, not busy-loop", calls)
	}

	sink.setBufferedAmount(0)
	sink.fireLow()

	waitForSentCount(t, sink, 1)

	got := sink.sentMessages()
	if string(got[0]) != "blocked" {
		t.Fatalf("sent %q, want %q", got[0], "blocked")
	}
}

// reassembleChunks strips each chunk's 2-byte header and concatenates the
// raw payload bytes -- exactly what the client's ChunkReassembler
// (client/services/webrtc-signal.ts) does. It is deliberately byte-level,
// never decoding a chunk to a string before the last one arrives: doing that
// instead would corrupt any multi-byte UTF-8 rune split across a chunk
// boundary, which is exactly what TestRTCConn_OversizedPayloadIsChunked
// below is designed to catch.
func reassembleChunks(t *testing.T, frames [][]byte) []byte {
	t.Helper()

	var out []byte

	for i, frame := range frames {
		if len(frame) < chunkHeaderSize {
			t.Fatalf("frame %d has length %d, shorter than the %d byte chunk header", i, len(frame), chunkHeaderSize)
		}

		if frame[0] != chunkMarker {
			t.Fatalf("frame %d marker byte = %#x, want chunkMarker %#x", i, frame[0], chunkMarker)
		}

		final := frame[1] == chunkFlagFinal
		if !final && i == len(frames)-1 {
			t.Fatalf("last frame (%d) was not flagged final", i)
		}
		if final && i != len(frames)-1 {
			t.Fatalf("frame %d flagged final but %d more frames follow", i, len(frames)-1-i)
		}

		out = append(out, frame[chunkHeaderSize:]...)
	}

	return out
}

// Criterion 1 (load-bearing on the Go side too): an over-ceiling payload is
// split into chunkMarker-framed pieces that reassemble, byte for byte, into
// the original payload -- including a multi-byte UTF-8 rune ('€', 3 bytes)
// deliberately placed to straddle a chunk boundary, so a reassembly that
// decoded each chunk to a string before concatenating (instead of joining
// raw bytes first) would be caught corrupting it.
func TestRTCConn_OversizedPayloadIsChunked(t *testing.T) {
	const ceiling = 10 // chunkHeaderSize(2) + 8 payload bytes per chunk

	sink := newFakeSink()
	conn := newConn("session-1", sink, ceiling)
	defer conn.Close()

	// Bytes 0-6 are ASCII filler (7 bytes); "€" is the 3-byte UTF-8 sequence
	// E2 82 AC, landing at offsets 7-9 -- one byte in the first 8-byte chunk,
	// two in the second. More filler follows to force a third chunk.
	payload := append([]byte("1234567"), []byte{0xE2, 0x82, 0xAC}...)
	payload = append(payload, []byte("more-tail-bytes-forcing-a-third-chunk")...)

	if !utf8.Valid(payload) {
		t.Fatal("test payload itself is not valid UTF-8 -- fix the test")
	}

	if err := conn.Write(payload); err != nil {
		t.Fatalf("Write(payload) returned error: %v", err)
	}

	wantChunks := (len(payload) + ceiling - chunkHeaderSize - 1) / (ceiling - chunkHeaderSize)
	waitForSentCount(t, sink, wantChunks)

	got := sink.sentMessages()
	if len(got) != wantChunks {
		t.Fatalf("sink received %d frames, want %d", len(got), wantChunks)
	}

	for i, frame := range got {
		if len(frame) > ceiling {
			t.Fatalf("frame %d is %d bytes, exceeds the %d byte ceiling", i, len(frame), ceiling)
		}
	}

	reassembled := reassembleChunks(t, got)
	if !bytes.Equal(reassembled, payload) {
		t.Fatalf("reassembled payload = %q, want %q (identical bytes)", reassembled, payload)
	}
}

// Criterion 4: a payload at or under the ceiling is written verbatim, with
// no chunk header and in exactly one sink.Send call.
func TestRTCConn_UnderCeilingPayloadSentUnchanged(t *testing.T) {
	const ceiling = 10

	sink := newFakeSink()
	conn := newConn("session-1", sink, ceiling)
	defer conn.Close()

	// Exactly at the ceiling: still unchanged, proving the boundary itself
	// isn't chunked.
	payload := []byte("0123456789")
	if len(payload) != ceiling {
		t.Fatalf("test payload is %d bytes, want exactly %d", len(payload), ceiling)
	}

	if err := conn.Write(payload); err != nil {
		t.Fatalf("Write(payload) returned error: %v", err)
	}

	waitForSentCount(t, sink, 1)
	time.Sleep(50 * time.Millisecond)

	got := sink.sentMessages()
	if len(got) != 1 {
		t.Fatalf("sink received %d messages, want exactly 1 (no chunking at or under the ceiling)", len(got))
	}

	if !bytes.Equal(got[0], payload) {
		t.Fatalf("sink received %q, want %q unchanged (no framing overhead)", got[0], payload)
	}
}

// Criterion 2: a nonzero negotiated max message size overrides the
// fallback -- a payload that would fit under fallbackMaxMessageSize is still
// chunked once it exceeds the smaller negotiated ceiling.
func TestRTCConn_NegotiatedMaxMessageSizeOverridesFallback(t *testing.T) {
	const negotiated = 32

	sink := newFakeSink()
	conn := newConn("session-1", sink, negotiated)
	defer conn.Close()

	if conn.maxMessageSize != negotiated {
		t.Fatalf("maxMessageSize = %d, want the negotiated value %d", conn.maxMessageSize, negotiated)
	}

	payload := bytes.Repeat([]byte("a"), negotiated+1)
	if err := conn.Write(payload); err != nil {
		t.Fatalf("Write(payload) returned error: %v", err)
	}

	// Well under fallbackMaxMessageSize, so this only chunks at all if the
	// negotiated ceiling -- not the fallback -- is what's in effect.
	waitForSentCount(t, sink, 2)

	got := sink.sentMessages()
	if len(got) != 2 {
		t.Fatalf("sink received %d frames, want 2", len(got))
	}

	if !bytes.Equal(reassembleChunks(t, got), payload) {
		t.Fatalf("reassembled payload does not match original")
	}
}

// Criterion 2 (fallback path): a zero negotiated size (no SCTP association
// available) falls back to fallbackMaxMessageSize, matching the pre-chunking
// ceiling exactly -- a payload one byte over it still chunks, one at it
// still doesn't.
func TestRTCConn_ZeroNegotiatedSizeFallsBackToDefault(t *testing.T) {
	sink := newFakeSink()
	conn := newConn("session-1", sink, 0)
	defer conn.Close()

	if conn.maxMessageSize != fallbackMaxMessageSize {
		t.Fatalf("maxMessageSize = %d, want fallbackMaxMessageSize %d", conn.maxMessageSize, fallbackMaxMessageSize)
	}

	oversized := bytes.Repeat([]byte("b"), fallbackMaxMessageSize+1)
	if err := conn.Write(oversized); err != nil {
		t.Fatalf("Write(oversized) returned error: %v", err)
	}

	waitForSentCount(t, sink, 2)

	got := sink.sentMessages()
	if len(got) != 2 {
		t.Fatalf("sink received %d frames, want 2", len(got))
	}

	for i, frame := range got {
		if len(frame) > fallbackMaxMessageSize {
			t.Fatalf("frame %d is %d bytes, exceeds fallbackMaxMessageSize %d", i, len(frame), fallbackMaxMessageSize)
		}
	}

	if !bytes.Equal(reassembleChunks(t, got), oversized) {
		t.Fatalf("reassembled payload does not match original")
	}
}

// A negotiated size too small to carry even one payload byte per chunk
// (at or below chunkHeaderSize) must not be trusted -- it would otherwise
// make sendChunked loop forever. newConn falls back to
// fallbackMaxMessageSize instead.
func TestRTCConn_TooSmallNegotiatedSizeFallsBackToDefault(t *testing.T) {
	conn := newConn("session-1", newFakeSink(), chunkHeaderSize)
	defer conn.Close()

	if conn.maxMessageSize != fallbackMaxMessageSize {
		t.Fatalf("maxMessageSize = %d, want fallbackMaxMessageSize %d for a negotiated size <= chunkHeaderSize", conn.maxMessageSize, fallbackMaxMessageSize)
	}
}

// Criterion 3 (Go side): closing the conn mid-chunk-sequence stops the drain
// goroutine instead of hanging or panicking, and does not push the rest of
// the in-flight sequence to the sink. Close cannot interrupt a Send already
// in flight (nothing short of the sink itself returning would), so this
// blocks the first chunk's Send, closes, and only then unblocks it -- proving
// the SECOND chunk, which sendChunked's done-check must refuse to start, is
// the one that never reaches the sink. The client-side counterpart -- proving
// the reassembly buffer itself is dropped, not just that sending stops --
// lives in webrtc-signal.node-test.ts.
func TestRTCConn_CloseMidChunkSequenceStopsCleanly(t *testing.T) {
	const ceiling = 10

	sink := newFakeSink()
	block := make(chan struct{})
	sink.setSendBlock(block)

	conn := newConn("session-1", sink, ceiling)

	payload := bytes.Repeat([]byte("c"), (ceiling-chunkHeaderSize)*5) // 5 chunks

	writeErr := make(chan error, 1)
	go func() { writeErr <- conn.Write(payload) }()

	select {
	case <-sink.sendStarted:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the first chunk's Send to start")
	}

	if err := conn.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	// Only now unblock the first chunk's already-in-flight Send: Close
	// cannot interrupt it, it can only stop the chunk after it from ever
	// starting.
	close(block)

	select {
	case <-conn.stopped:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the drain goroutine to stop after Close")
	}

	if err := <-writeErr; err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	// Give the drain goroutine a moment to see if it (incorrectly) sends
	// more than the one chunk that was already in flight when Close ran.
	time.Sleep(50 * time.Millisecond)

	if got := len(sink.sentMessages()); got != 1 {
		t.Fatalf("sink received %d of 5 chunks after a mid-sequence Close, want exactly 1 (the chunk already in flight)", got)
	}
}
