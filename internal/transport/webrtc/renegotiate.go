package webrtc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	pion "github.com/pion/webrtc/v4"
)

// This file is the spike for plan Open Question P1 Q1 (story 041-e060): can a
// PeerConnection negotiated for a DataChannel only later carry a track added
// after connect? It ships only what answers that question -- serialised
// renegotiation over the "signal" channel -- and nothing else. No capture, no
// routing, no SFU.
//
// Design note on OnNegotiationNeeded: pion fires that callback whenever
// AddTrack (or any other change) sets the negotiation-needed flag, and it
// must be registered before the PeerConnection is established or an early
// firing is missed. This spike does not rely on it: negotiate below drives
// AddTrack and the offer/answer exchange as one explicit, directly-callable
// sequence, which is what makes the serialisation assertion in
// renegotiation_test.go deterministic rather than dependent on when pion
// happens to invoke an async callback.

// setSignal records p's "signal" DataChannel so negotiate has something to
// send an offer over. Guarded by signalMu because attachSignal runs on
// pion's DataChannel callback goroutine and can race a renegotiation already
// waiting to send (or about to start).
func (p *peer) setSignal(dc *pion.DataChannel) {
	p.signalMu.Lock()
	p.signal = dc
	p.signalMu.Unlock()
}

// handleSignalMessage is the "signal" channel's OnMessage handler. It decodes
// an inbound Signal and, if a renegotiation is currently awaiting an answer,
// delivers it there. Reusing DecodeSignal (signal.go) keeps the same size cap
// and shape validation this envelope already gets on the initial offer path.
func (p *peer) handleSignalMessage(data []byte) {
	signal, err := DecodeSignal(bytes.NewReader(data))
	if err != nil {
		log.Printf("webrtc: decoding signal channel message: %v", err)

		return
	}

	p.signalMu.Lock()
	pending := p.pending
	p.signalMu.Unlock()

	if pending == nil {
		log.Printf("webrtc: received a %q signal message with no renegotiation in flight", signal.Type)

		return
	}

	select {
	case pending <- *signal:
	default:
		// pending is buffered by one and has exactly one writer -- this
		// handler -- while negotiate holds p.negotiating, so a full channel
		// here means a second message arrived after an answer was already
		// delivered. Not worth blocking the DataChannel callback goroutine
		// over; log and move on.
		log.Printf("webrtc: dropping signal message %q; an answer was already delivered", signal.Type)
	}
}

// negotiate is the whole spike: AddTrack, then drive one offer/answer cycle
// for it over the signal channel, and report whether it worked.
//
// One renegotiation at a time per peer: overlapping AddTrack calls produce
// "Failed to process the bundled m= section" (pion issue #1169). The lock
// brackets the entire AddTrack -> offer -> answer cycle, not just the network
// round trip, so a second concurrent caller blocks until this one's
// SetRemoteDescription has landed.
func (p *peer) negotiate(ctx context.Context, track pion.TrackLocal) (*pion.RTPSender, error) {
	p.negotiating.Lock()
	defer p.negotiating.Unlock()

	sender, err := p.pc.AddTrack(track)
	if err != nil {
		return nil, fmt.Errorf("adding track: %w", err)
	}

	if err := p.offerAndAwaitAnswer(ctx); err != nil {
		return nil, err
	}

	return sender, nil
}

// offerAndAwaitAnswer runs CreateOffer -> SetLocalDescription -> send over
// the signal channel -> await the answer -> SetRemoteDescription. Called only
// from negotiate, which already holds p.negotiating.
func (p *peer) offerAndAwaitAnswer(ctx context.Context) error {
	offer, err := p.pc.CreateOffer(nil)
	if err != nil {
		return fmt.Errorf("creating offer: %w", err)
	}

	if err := p.pc.SetLocalDescription(offer); err != nil {
		return fmt.Errorf("setting local description: %w", err)
	}

	answerCh := make(chan Signal, 1)

	p.signalMu.Lock()
	signal := p.signal
	p.pending = answerCh
	p.signalMu.Unlock()

	defer func() {
		p.signalMu.Lock()
		p.pending = nil
		p.signalMu.Unlock()
	}()

	if signal == nil {
		return errors.New("renegotiating: signal channel is not open")
	}

	local := p.pc.LocalDescription()

	body, err := json.Marshal(Signal{Type: "offer", SDP: local.SDP})
	if err != nil {
		return fmt.Errorf("marshalling renegotiation offer: %w", err)
	}

	if err := signal.SendText(string(body)); err != nil {
		return fmt.Errorf("sending renegotiation offer over signal channel: %w", err)
	}

	var answer Signal

	select {
	case answer = <-answerCh:
	case <-ctx.Done():
		return fmt.Errorf("awaiting renegotiation answer: %w", ctx.Err())
	}

	if answer.Type != "answer" {
		return fmt.Errorf("renegotiating: expected an answer over the signal channel, got %q", answer.Type)
	}

	if err := p.pc.SetRemoteDescription(pion.SessionDescription{Type: pion.SDPTypeAnswer, SDP: answer.SDP}); err != nil {
		return fmt.Errorf("setting remote description from renegotiation answer: %w", err)
	}

	return nil
}
