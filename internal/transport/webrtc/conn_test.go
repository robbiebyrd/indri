package webrtc

import (
	"bytes"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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
	conn := newConn("session-1", newFakeSink())
	defer conn.Close()

	var _ *transport.BufferedConn[[]byte] = conn.BufferedConn

	var _ transport.Conn = conn
}

// Criterion 2: writes reach the sink in order.
func TestRTCConn_WritesReachSinkInOrder(t *testing.T) {
	sink := newFakeSink()
	conn := newConn("session-1", sink)
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

	conn := newConn("session-1", sink)
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
	conn := newConn("session-1", newFakeSink())

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
	conn := newConn("session-1", newFakeSink())

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

	conn := newConn("session-1", sink)
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

// Criterion 7 (story 040-9cec): a payload over the practical cross-browser
// DataChannel ceiling is reported and logged, never silently truncated (which
// pion would otherwise hand to the sink, risking corruption) and never
// silently dropped with no trace (which would leave a player's client simply
// frozen with nothing anywhere to explain why).
func TestRTCConn_OversizedPayloadReportedNotSent(t *testing.T) {
	var logBuf bytes.Buffer

	log.SetOutput(&logBuf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	sink := newFakeSink()
	conn := newConn("session-1", sink)
	defer conn.Close()

	oversized := make([]byte, maxMessageSize+1)

	if err := conn.Write(oversized); err != nil {
		t.Fatalf("Write(oversized) returned error: %v", err)
	}

	// Follow the oversized payload with a normal one so we can prove the
	// drain goroutine kept going instead of stalling on it.
	if err := conn.Write([]byte("ok")); err != nil {
		t.Fatalf("Write(ok) returned error: %v", err)
	}

	waitForSentCount(t, sink, 1)

	// Give the drain goroutine a moment to see if it (incorrectly) also
	// forwards the oversized payload.
	time.Sleep(50 * time.Millisecond)

	got := sink.sentMessages()
	if len(got) != 1 || string(got[0]) != "ok" {
		t.Fatalf("sink received %q, want exactly one message %q -- the oversized payload must never reach the sink", got, "ok")
	}

	logged := logBuf.String()
	if !strings.Contains(logged, strconv.Itoa(len(oversized))) || !strings.Contains(logged, strconv.Itoa(maxMessageSize)) {
		t.Errorf("log output = %q, want it to report both the oversized payload's size (%d) and the ceiling (%d)",
			logged, len(oversized), maxMessageSize)
	}
}
