package webrtc

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"

	pionice "github.com/pion/ice/v4"
	pion "github.com/pion/webrtc/v4"
)

// Config configures the pion API and UDP mux shared by every peer this
// transport creates.
type Config struct {
	// UDPPort is the single port the ICE mux binds for every peer this
	// transport creates. A range would not constrain server-reflexive
	// candidate ports, so it cannot actually limit exposure; one fixed port
	// is what makes container port mapping possible. Pass 0 to let the OS
	// assign a free port -- tests must do this so they never collide with a
	// developer's already-running server.
	UDPPort int

	// NAT1To1IPs are advertised as host candidates in place of the machine's
	// private address, so a container behind a 1:1 NAT (e.g. a docker port
	// mapping, or an AWS EIP) is reachable at its actually-routable address.
	// Empty disables the rewrite.
	NAT1To1IPs []string
}

// newAPI builds the pion API and UDP mux shared by every PeerConnection this
// transport creates. Call it exactly once per transport and reuse the
// returned API for every peer -- see the RegisterDefaultCodecs comment below
// for why a second call is unsafe.
//
// The caller owns the returned mux's lifecycle (Close it on transport
// shutdown); that wiring belongs to the transport type built on top of this
// file, not here.
func newAPI(cfg Config) (*pion.API, *pionice.MultiUDPMuxDefault, error) {
	mediaEngine := &pion.MediaEngine{}

	// RegisterDefaultCodecs is the one thing that cannot be added after a
	// PeerConnection exists: a PeerConnection negotiates only the codecs its
	// MediaEngine held at creation time. No media ships yet, but registering
	// now is what keeps audio/video possible later at zero cost today.
	// RegisterDefaultCodecs is documented as unsafe for concurrent use, so it
	// must run here, once, synchronously, before any PeerConnection is built
	// from the API this returns.
	if err := mediaEngine.RegisterDefaultCodecs(); err != nil {
		return nil, nil, fmt.Errorf("registering default codecs: %w", err)
	}

	// The mux must exist before any PeerConnection uses it: SettingEngine
	// only stores a reference to it, and every peer built from the returned
	// API shares this single UDP port -- not an ephemeral range, which would
	// not actually constrain the server-reflexive candidate ports a peer
	// advertises. UDPMuxFromPortWithLoopback lets a peer on the loopback
	// interface (in-process tests, or a VM with a public IP mapped to
	// loopback) gather candidates that are actually reachable.
	mux, err := pionice.NewMultiUDPMuxFromPort(cfg.UDPPort, pionice.UDPMuxFromPortWithLoopback())
	if err != nil {
		return nil, nil, fmt.Errorf("creating udp mux on port %d: %w", cfg.UDPPort, err)
	}

	settingEngine := pion.SettingEngine{}
	settingEngine.SetICEUDPMux(mux)

	if len(cfg.NAT1To1IPs) > 0 {
		// ICECandidateTypeHost replaces the advertised host address outright.
		// That is what a container needs: its private address is otherwise
		// unreachable and would just confuse a remote peer's ICE checks.
		settingEngine.SetNAT1To1IPs(cfg.NAT1To1IPs, pion.ICECandidateTypeHost)
	}

	api := pion.NewAPI(pion.WithMediaEngine(mediaEngine), pion.WithSettingEngine(settingEngine))

	return api, mux, nil
}

// answerOffer turns a client's non-trickle offer into a complete, non-trickle
// SDP answer: apply the offer, create and set this side's answer, then wait
// for this side's own ICE gathering to finish so the SDP handed back already
// carries every candidate. That is the whole point of non-trickle signalling
// -- one HTTP round trip, no push channel, no second endpoint.
func answerOffer(
	ctx context.Context,
	pc *pion.PeerConnection,
	offer pion.SessionDescription,
) (*pion.SessionDescription, error) {
	if err := pc.SetRemoteDescription(offer); err != nil {
		return nil, fmt.Errorf("setting remote description: %w", err)
	}

	localAnswer, err := pc.CreateAnswer(nil)
	if err != nil {
		return nil, fmt.Errorf("creating answer: %w", err)
	}

	// Register the gather-complete promise before SetLocalDescription:
	// GatheringCompletePromise itself guards the race of gathering finishing
	// before the handler is registered, but only if it is called first.
	gatherComplete := pion.GatheringCompletePromise(pc)

	if err := pc.SetLocalDescription(localAnswer); err != nil {
		return nil, fmt.Errorf("setting local description: %w", err)
	}

	// Never a bare <-gatherComplete: pion issue #2507 leaks this goroutine
	// forever if the PeerConnection closes before gathering finishes. ctx
	// bounds the wait so a misconfigured SetNAT1To1IPs (or a peer closing
	// mid-gather) fails the signalling request instead of hanging it.
	select {
	case <-gatherComplete:
	case <-ctx.Done():
		return nil, fmt.Errorf("ice gathering did not complete: %w", ctx.Err())
	}

	// pion's own ICEGatherer.Close fires this same gatherComplete handler as
	// a leak guard, precisely so a bare receive here would not hang -- but
	// that firing means "stop waiting", not "gathering actually finished".
	// Without this check a peer closed mid-gather would fall through as a
	// false success, handing back a stale, incomplete answer.
	if pc.SignalingState() == pion.SignalingStateClosed {
		return nil, errors.New("ice gathering did not complete: peer connection closed")
	}

	return pc.LocalDescription(), nil
}

// peer owns one PeerConnection plus the rtcConn built on its DataChannel, and
// guarantees the pair is torn down exactly once no matter which callback
// notices the connection died.
//
// peer deliberately does not know about the Registry that will eventually
// hold every peer (that type is introduced by the transport in a later
// story): onClose is a callback supplied by the constructor's caller, who is
// responsible for deregistering the peer. peer's only job is to run that
// callback, and close its own resources, exactly once.
type peer struct {
	pc   *pion.PeerConnection
	conn *rtcConn

	// teardown guards close so it runs exactly once regardless of which
	// notification path gets there first -- see handleConnectionStateChange.
	teardown sync.Once

	// onClose deregisters this peer from whatever holds it (the Registry,
	// once story 039 introduces one). May be nil for a peer that has not been
	// registered anywhere yet.
	onClose func()
}

// newPeer wires pc's connection-state callback to teardown and returns the
// peer. conn may be nil for a peer that never reached an open DataChannel
// (e.g. torn down by the pending-peer TTL before the client sent anything);
// close tolerates that.
func newPeer(pc *pion.PeerConnection, conn *rtcConn, onClose func()) *peer {
	p := &peer{pc: pc, conn: conn, onClose: onClose}
	pc.OnConnectionStateChange(p.handleConnectionStateChange)

	return p
}

// handleConnectionStateChange is the pure decision of which states tear the
// peer down. pion invokes it through the callback registered in newPeer, but
// it is unexported and side-effect-free enough for tests to call directly,
// which is what makes criteria 1-4 deterministic instead of dependent on a
// real ICE failure.
func (p *peer) handleConnectionStateChange(state pion.PeerConnectionState) {
	switch state {
	case pion.PeerConnectionStateFailed, pion.PeerConnectionStateClosed:
		// Failed and Closed are the only terminal states. Disconnected is
		// transient and recovers within pion's ~30s default window (see the
		// plan's Research Findings); tearing down on it would drop players
		// who would otherwise have reconnected.
		p.teardown.Do(p.close)
	case pion.PeerConnectionStateDisconnected:
		// Deliberately not a teardown trigger -- see the comment above.
	}
}

// close releases the peer's resources exactly once. It is only ever invoked
// through p.teardown.Do: OnConnectionStateChange fires on a fresh goroutine
// per invocation, while OnICEConnectionStateChange runs synchronously, with
// no ordering guarantee between them, so two independent notification paths
// can both observe the failure and race to tear down (pion issue #744).
// sync.Once is what makes that race harmless rather than a double-close.
func (p *peer) close() {
	// pc.Close is idempotent (pion swaps an isClosed flag under its own
	// lock and returns nil if it was already set), so calling it again when
	// the state is already Closed is a safe no-op. It still must run for a
	// Failed peer, which pion does not close automatically.
	if err := p.pc.Close(); err != nil {
		log.Printf("webrtc: closing peer connection during teardown: %v", err)
	}

	if p.conn != nil {
		if err := p.conn.Close(); err != nil {
			log.Printf("webrtc: closing conn during peer teardown: %v", err)
		}
	}

	if p.onClose != nil {
		p.onClose()
	}
}
