package webrtc

import (
	"bytes"
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	pion "github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

// opusCapability mirrors exactly the Opus RTPCodecCapability that newAPI's
// MediaEngine registers via RegisterDefaultCodecs, so the renegotiation
// offer this spike creates negotiates against a codec the server already
// knows about.
var opusCapability = pion.RTPCodecCapability{
	MimeType:    pion.MimeTypeOpus,
	ClockRate:   48000,
	Channels:    2,
	SDPFmtpLine: "minptime=10;useinbandfec=1",
}

// serverPeer returns the single peer this test's transport currently holds.
// Reading tr.peers directly is safe here: this file is in the same package
// as webrtc.go/peer.go, and every caller reaches this only after
// clientHandshake has already completed the offer/answer exchange that
// creates the peer.
func serverPeer(t *testing.T, tr *Transport) *peer {
	t.Helper()

	tr.mu.Lock()
	defer tr.mu.Unlock()

	for _, p := range tr.peers {
		if p != nil {
			return p
		}
	}

	t.Fatal("no peer registered on the transport")

	return nil
}

// answerRenegotiationOffers wires ch's OnMessage handler to play the client
// side of a renegotiation: any inbound offer is applied, answered, and the
// answer is sent back over the same channel. before/after bracket the SDP
// exchange so a test can observe how many offers are being processed at
// once (criterion 3); either may be nil.
func answerRenegotiationOffers(t *testing.T, clientPC *pion.PeerConnection, ch *pion.DataChannel, before, after func()) {
	t.Helper()

	ch.OnMessage(func(msg pion.DataChannelMessage) {
		signal, err := DecodeSignal(bytes.NewReader(msg.Data))
		if err != nil || signal.Type != "offer" {
			// Not every message on this channel is necessarily a
			// renegotiation offer (e.g. a stray non-offer Signal); ignore
			// rather than fail the test from inside a callback.
			return
		}

		if before != nil {
			before()
		}

		if after != nil {
			defer after()
		}

		if err := clientPC.SetRemoteDescription(pion.SessionDescription{Type: pion.SDPTypeOffer, SDP: signal.SDP}); err != nil {
			t.Errorf("client SetRemoteDescription(offer): %v", err)

			return
		}

		answer, err := clientPC.CreateAnswer(nil)
		if err != nil {
			t.Errorf("client CreateAnswer: %v", err)

			return
		}

		if err := clientPC.SetLocalDescription(answer); err != nil {
			t.Errorf("client SetLocalDescription(answer): %v", err)

			return
		}

		body, err := json.Marshal(Signal{Type: "answer", SDP: clientPC.LocalDescription().SDP})
		if err != nil {
			t.Errorf("marshal answer: %v", err)

			return
		}

		if err := ch.SendText(string(body)); err != nil {
			t.Errorf("SendText(answer): %v", err)
		}
	})
}

// Criteria 1, 2 and 6 -- plan Open Question P1 Q1: after the DataChannel is
// open, the server calls AddTrack and drives the offer/answer over the
// signal channel; the in-process pion client either fires OnTrack or the
// negative result IS the finding. This test is written to assert whichever
// behaviour was actually observed on the pion version below (see the
// plan's Open Questions for the recorded finding), so a future pion upgrade
// that changes this behaviour fails it loudly instead of silently.
//
// pion version at time of finding: github.com/pion/webrtc/v4 v4.2.20.
func TestRenegotiate_TrackAddedAfterDataChannelOnly(t *testing.T) {
	tr := newTestTransport(t, noSessions{})
	url := newTestServer(t, tr)

	clientPC, channels := clientHandshake(t, url, "", gameChannel, signalChannel)

	trackCh := make(chan *pion.TrackRemote, 1)
	clientPC.OnTrack(func(track *pion.TrackRemote, _ *pion.RTPReceiver) {
		select {
		case trackCh <- track:
		default:
		}
	})

	answerRenegotiationOffers(t, clientPC, channels[signalChannel], nil, nil)

	p := serverPeer(t, tr)

	track, err := pion.NewTrackLocalStaticSample(opusCapability, "audio", "spike-041")
	if err != nil {
		t.Fatalf("NewTrackLocalStaticSample: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	if _, err := p.negotiate(ctx, track); err != nil {
		t.Fatalf(
			"negotiate: %v -- renegotiation itself failed to complete, a stronger negative than issues "+
				"#1073/#2774 (those report a completed renegotiation whose OnTrack never fires)", err,
		)
	}

	// Issues #1073/#2774 report OnTrack failing to fire even after packets
	// are sent, so this gives the client real RTP to receive rather than
	// stopping at a completed SDP exchange -- a bounded, repeated write so a
	// single dropped/early packet on loopback can't produce a false
	// negative.
	var received *pion.TrackRemote

	deadline := time.Now().Add(testTimeout)
	for received == nil && time.Now().Before(deadline) {
		if err := track.WriteSample(media.Sample{Data: []byte{0xFC, 0xFF, 0xFE}, Duration: 20 * time.Millisecond}); err != nil {
			t.Fatalf("WriteSample: %v", err)
		}

		select {
		case received = <-trackCh:
		case <-time.After(20 * time.Millisecond):
		}
	}

	if received == nil {
		t.Fatal(
			"FINDING (pion v4.2.20): renegotiation completed -- offer/answer exchanged and applied -- but " +
				"OnTrack never fired on the client after repeated RTP writes. Reproduces pion issues " +
				"#1073/#2774 for a DataChannel-only PeerConnection upgraded to carry media. Phase-two video " +
				"needs a second PeerConnection (or an SFU such as LiveKit), not this connection.",
		)
	}

	t.Logf(
		"FINDING (pion v4.2.20): OnTrack fired for a track added after the DataChannel-only PeerConnection "+
			"was already connected (kind=%s, mime=%s). A single PeerConnection can carry a track added after "+
			"connect for this configuration.",
		received.Kind(), received.Codec().MimeType,
	)
}

// Criterion 3: a second AddTrack issued while the first negotiation is in
// flight must be serialised, not interleaved -- overlapping AddTrack calls
// produce "Failed to process the bundled m= section" (pion issue #1169).
// This does not assert on timing: it counts how many renegotiation offers
// the client is processing at once and requires the peak to be exactly 1,
// with both offers still arriving (peak concurrency, not offer count, is
// where an unserialised implementation would show 2).
func TestPeerNegotiate_SerialisesConcurrentRenegotiations(t *testing.T) {
	tr := newTestTransport(t, noSessions{})
	url := newTestServer(t, tr)

	clientPC, channels := clientHandshake(t, url, "", gameChannel, signalChannel)

	var (
		mu      sync.Mutex
		active  int
		maxSeen int
		offers  int
	)

	enter := func() {
		mu.Lock()
		active++
		offers++

		if active > maxSeen {
			maxSeen = active
		}

		mu.Unlock()
	}

	leave := func() {
		mu.Lock()
		active--
		mu.Unlock()
	}

	answerRenegotiationOffers(t, clientPC, channels[signalChannel], enter, leave)

	p := serverPeer(t, tr)

	trackA, err := pion.NewTrackLocalStaticSample(opusCapability, "audio", "spike-041-a")
	if err != nil {
		t.Fatalf("NewTrackLocalStaticSample(a): %v", err)
	}

	trackB, err := pion.NewTrackLocalStaticSample(opusCapability, "audio", "spike-041-b")
	if err != nil {
		t.Fatalf("NewTrackLocalStaticSample(b): %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var wg sync.WaitGroup

	errs := make(chan error, 2)
	wg.Add(2)

	go func() {
		defer wg.Done()

		_, err := p.negotiate(ctx, trackA)
		errs <- err
	}()

	go func() {
		defer wg.Done()

		_, err := p.negotiate(ctx, trackB)
		errs <- err
	}()

	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("negotiate: %v", err)
		}
	}

	mu.Lock()
	defer mu.Unlock()

	if offers != 2 {
		t.Fatalf("client processed %d renegotiation offers, want 2 (both AddTrack calls must still complete)", offers)
	}

	if maxSeen != 1 {
		t.Fatalf(
			"peak concurrent renegotiation offers being processed = %d, want 1 -- negotiate did not serialise "+
				"overlapping AddTrack calls (pion issue #1169)", maxSeen,
		)
	}
}
