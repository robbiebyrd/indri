package webrtc

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	pion "github.com/pion/webrtc/v4"
)

// testTimeout bounds every gather/handshake wait in this file. Loopback ICE
// is real UDP, so it's generous, but every wait here is a select against a
// context or a time.After -- never a bare receive that could block forever.
const testTimeout = 5 * time.Second

// sdpCandidate is one parsed "a=candidate:" attribute line
// (RFC 5245 section 15.1): candidate:<foundation> <component> <transport>
// <priority> <address> <port> typ <type> ...
type sdpCandidate struct {
	address string
	port    string
	typ     string
}

// parseCandidates extracts every ICE candidate line from an SDP string.
// Tests assert on the SDP text directly, rather than on OnICECandidate
// callback timing, because the whole point of a non-trickle answer is that
// the candidates are already in the SDP by the time the caller sees it.
func parseCandidates(t *testing.T, sdpText string) []sdpCandidate {
	t.Helper()

	var out []sdpCandidate

	for _, line := range strings.Split(sdpText, "\r\n") {
		if !strings.HasPrefix(line, "a=candidate:") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 8 {
			t.Fatalf("malformed candidate line %q", line)
		}

		out = append(out, sdpCandidate{address: fields[4], port: fields[5], typ: fields[7]})
	}

	return out
}

// gatherLocalOffer creates an offer for pc, sets it as the local description,
// and waits (bounded by ctx) for ICE gathering to finish before returning it.
// The caller must already have added a data channel or transceiver to pc --
// gathering needs something to negotiate.
func gatherLocalOffer(t *testing.T, ctx context.Context, pc *pion.PeerConnection) pion.SessionDescription {
	t.Helper()

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}

	gatherComplete := pion.GatheringCompletePromise(pc)

	if err := pc.SetLocalDescription(offer); err != nil {
		t.Fatalf("SetLocalDescription: %v", err)
	}

	select {
	case <-gatherComplete:
	case <-ctx.Done():
		t.Fatalf("gathering did not complete: %v", ctx.Err())
	}

	return *pc.LocalDescription()
}

// Criterion 1: a single webrtc.API is built once per transport with
// RegisterDefaultCodecs called exactly once.
func TestNewAPI_RegistersCodecs(t *testing.T) {
	api, mux, err := newAPI(Config{UDPPort: 0})
	if err != nil {
		t.Fatalf("newAPI: %v", err)
	}
	t.Cleanup(func() { _ = mux.Close() })

	pc, err := api.NewPeerConnection(pion.Configuration{})
	if err != nil {
		t.Fatalf("NewPeerConnection: %v", err)
	}
	t.Cleanup(func() { _ = pc.Close() })

	// AddTransceiverFromKind fails with ErrNoCodecsAvailable unless the
	// MediaEngine backing this API registered codecs at construction --
	// the only way to observe registration from outside the pion package.
	if _, err := pc.AddTransceiverFromKind(pion.RTPCodecTypeAudio); err != nil {
		t.Fatalf("AddTransceiverFromKind: %v -- codecs were not registered by newAPI", err)
	}
}

// freeUDPPort reserves an OS-assigned UDP port and immediately releases it, so
// the mux can bind that one concrete port on every interface.
//
// Port 0 will not do for this test: the mux binds one socket per interface, and
// with 0 each socket gets a DIFFERENT ephemeral port — exactly the situation
// this test exists to rule out. Whether it passed would then depend on how many
// interfaces happen to be up.
func freeUDPPort(t *testing.T) int {
	t.Helper()

	probe, err := net.ListenUDP("udp", &net.UDPAddr{})
	if err != nil {
		t.Fatalf("reserving a udp port: %v", err)
	}

	port := probe.LocalAddr().(*net.UDPAddr).Port

	if err := probe.Close(); err != nil {
		t.Fatalf("releasing the probe port: %v", err)
	}

	return port
}

// Criterion 2: all peers share one UDP port via a mux created before any
// PeerConnection uses it.
func TestNewAPI_PeersShareUDPPort(t *testing.T) {
	api, mux, err := newAPI(Config{UDPPort: freeUDPPort(t)})
	if err != nil {
		t.Fatalf("newAPI: %v", err)
	}
	t.Cleanup(func() { _ = mux.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	pc1, err := api.NewPeerConnection(pion.Configuration{})
	if err != nil {
		t.Fatalf("pc1 NewPeerConnection: %v", err)
	}
	t.Cleanup(func() { _ = pc1.Close() })

	pc2, err := api.NewPeerConnection(pion.Configuration{})
	if err != nil {
		t.Fatalf("pc2 NewPeerConnection: %v", err)
	}
	t.Cleanup(func() { _ = pc2.Close() })

	if _, err := pc1.CreateDataChannel("probe", nil); err != nil {
		t.Fatalf("pc1 CreateDataChannel: %v", err)
	}

	if _, err := pc2.CreateDataChannel("probe", nil); err != nil {
		t.Fatalf("pc2 CreateDataChannel: %v", err)
	}

	offer1 := gatherLocalOffer(t, ctx, pc1)
	offer2 := gatherLocalOffer(t, ctx, pc2)

	candidates1 := parseCandidates(t, offer1.SDP)
	candidates2 := parseCandidates(t, offer2.SDP)

	if len(candidates1) == 0 || len(candidates2) == 0 {
		t.Fatalf("expected at least one candidate per peer, got %d and %d", len(candidates1), len(candidates2))
	}

	muxPorts := make(map[string]bool)

	for _, addr := range mux.GetListenAddresses() {
		if udpAddr, ok := addr.(*net.UDPAddr); ok {
			muxPorts[strconv.Itoa(udpAddr.Port)] = true
		}
	}

	for _, c := range append(append([]sdpCandidate{}, candidates1...), candidates2...) {
		if !muxPorts[c.port] {
			t.Fatalf("candidate port %s is not one of the mux's listen ports %v", c.port, muxPorts)
		}
	}

	// Not just "some port from the mux's pool" -- the very same port, since
	// there is exactly one.
	if candidates1[0].port != candidates2[0].port {
		t.Fatalf("peer1 and peer2 used different ports (%s vs %s); want the single shared mux port",
			candidates1[0].port, candidates2[0].port)
	}
}

// Criterion 3: SetNAT1To1IPs is applied when configured.
func TestNewAPI_NAT1To1IPsApplied(t *testing.T) {
	const publicIP = "203.0.113.5" // TEST-NET-3 (RFC 5737): never a real route.

	api, mux, err := newAPI(Config{UDPPort: 0, NAT1To1IPs: []string{publicIP}})
	if err != nil {
		t.Fatalf("newAPI: %v", err)
	}
	t.Cleanup(func() { _ = mux.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	pc, err := api.NewPeerConnection(pion.Configuration{})
	if err != nil {
		t.Fatalf("NewPeerConnection: %v", err)
	}
	t.Cleanup(func() { _ = pc.Close() })

	if _, err := pc.CreateDataChannel("probe", nil); err != nil {
		t.Fatalf("CreateDataChannel: %v", err)
	}

	offer := gatherLocalOffer(t, ctx, pc)
	candidates := parseCandidates(t, offer.SDP)

	found := false

	for _, c := range candidates {
		if c.address == publicIP && c.typ == "host" {
			found = true

			break
		}
	}

	if !found {
		t.Fatalf("candidates %+v do not include configured NAT1To1IP %s as a host candidate", candidates, publicIP)
	}
}

// Criterion 4: an offer produces an answer whose SDP already contains
// candidates.
func TestAnswerOffer_SDPContainsCandidates(t *testing.T) {
	api, mux, err := newAPI(Config{UDPPort: 0})
	if err != nil {
		t.Fatalf("newAPI: %v", err)
	}
	t.Cleanup(func() { _ = mux.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// A bare pion PeerConnection with default settings plays the client --
	// exactly what a real browser peer looks like from this side.
	clientPC, err := pion.NewPeerConnection(pion.Configuration{})
	if err != nil {
		t.Fatalf("client NewPeerConnection: %v", err)
	}
	t.Cleanup(func() { _ = clientPC.Close() })

	if _, err := clientPC.CreateDataChannel("game", nil); err != nil {
		t.Fatalf("CreateDataChannel: %v", err)
	}

	offer := gatherLocalOffer(t, ctx, clientPC)

	serverPC, err := api.NewPeerConnection(pion.Configuration{})
	if err != nil {
		t.Fatalf("server NewPeerConnection: %v", err)
	}
	t.Cleanup(func() { _ = serverPC.Close() })

	answer, err := answerOffer(ctx, serverPC, offer)
	if err != nil {
		t.Fatalf("answerOffer: %v", err)
	}

	candidates := parseCandidates(t, answer.SDP)
	if len(candidates) == 0 {
		t.Fatal("answer SDP has no candidates; a non-trickle answer must already be fully gathered")
	}
}

// Criterion 5: the gathering wait selects against a context and is never a
// bare channel receive. A context cancelled before answerOffer is ever
// called proves the ctx.Done() branch is live and reachable -- a bare
// <-gatherComplete would ignore it and instead wait for real gathering.
func TestAnswerOffer_HonorsContextDeadline(t *testing.T) {
	api, mux, err := newAPI(Config{UDPPort: 0})
	if err != nil {
		t.Fatalf("newAPI: %v", err)
	}
	t.Cleanup(func() { _ = mux.Close() })

	gatherCtx, gatherCancel := context.WithTimeout(context.Background(), testTimeout)
	defer gatherCancel()

	clientPC, err := pion.NewPeerConnection(pion.Configuration{})
	if err != nil {
		t.Fatalf("client NewPeerConnection: %v", err)
	}
	t.Cleanup(func() { _ = clientPC.Close() })

	if _, err := clientPC.CreateDataChannel("game", nil); err != nil {
		t.Fatalf("CreateDataChannel: %v", err)
	}

	offer := gatherLocalOffer(t, gatherCtx, clientPC)

	serverPC, err := api.NewPeerConnection(pion.Configuration{})
	if err != nil {
		t.Fatalf("server NewPeerConnection: %v", err)
	}
	t.Cleanup(func() { _ = serverPC.Close() })

	// Already cancelled before answerOffer is called at all.
	expiredCtx, expiredCancel := context.WithCancel(context.Background())
	expiredCancel()

	start := time.Now()

	_, err = answerOffer(expiredCtx, serverPC, offer)

	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected an error from an already-cancelled context")
	}

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	if elapsed > 500*time.Millisecond {
		t.Fatalf("answerOffer took %v to honor a cancelled context; want near-instant", elapsed)
	}
}

// Criterion 6: a PeerConnection closed mid-gather returns an error rather
// than hanging. This is a bounded test (select against time.After) so it
// fails loudly instead of blocking forever if the fix regresses.
func TestAnswerOffer_ClosedMidGatherReturnsError(t *testing.T) {
	api, mux, err := newAPI(Config{UDPPort: 0})
	if err != nil {
		t.Fatalf("newAPI: %v", err)
	}
	t.Cleanup(func() { _ = mux.Close() })

	gatherCtx, gatherCancel := context.WithTimeout(context.Background(), testTimeout)
	defer gatherCancel()

	clientPC, err := pion.NewPeerConnection(pion.Configuration{})
	if err != nil {
		t.Fatalf("client NewPeerConnection: %v", err)
	}
	t.Cleanup(func() { _ = clientPC.Close() })

	if _, err := clientPC.CreateDataChannel("game", nil); err != nil {
		t.Fatalf("CreateDataChannel: %v", err)
	}

	offer := gatherLocalOffer(t, gatherCtx, clientPC)

	// An unreachable STUN server (TEST-NET-3, RFC 5737) gives gathering a
	// server-reflexive candidate it can never resolve, so the server's own
	// host candidates finish almost instantly but ICEGatheringStateComplete
	// does not -- without this, host-only gathering on loopback completes in
	// well under a millisecond and the close below would race a gather that
	// has usually already finished, rather than exercising the mid-gather
	// path pion issue #2507 is about.
	serverPC, err := api.NewPeerConnection(pion.Configuration{
		ICEServers: []pion.ICEServer{{URLs: []string{"stun:203.0.113.1:19302"}}},
	})
	if err != nil {
		t.Fatalf("server NewPeerConnection: %v", err)
	}
	// Not deferred: the test closes serverPC explicitly, mid-flight.

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan error, 1)

	go func() {
		_, answerErr := answerOffer(ctx, serverPC, offer)
		done <- answerErr
	}()

	// Race the close against the gather. The unreachable STUN server keeps
	// gathering open for seconds, so this window is real, not a coin flip.
	// Whichever way answerOffer observes the close -- an error from
	// CreateAnswer/SetLocalDescription on an already-closed PeerConnection,
	// or ctx expiring because gathering can never finish on a closed
	// connection -- it must return, not hang (pion #2507).
	time.Sleep(20 * time.Millisecond)

	if err := serverPC.Close(); err != nil {
		t.Fatalf("closing server PeerConnection: %v", err)
	}

	select {
	case answerErr := <-done:
		if answerErr == nil {
			t.Fatal("expected an error from answerOffer after the PeerConnection closed mid-gather")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("answerOffer did not return after the PeerConnection was closed mid-gather (goroutine leak, pion #2507)")
	}
}

// Criterion 7: an in-process test drives both peers with pion and exchanges
// messages both ways.
func TestAnswerOffer_DataChannelMessagesFlowBothWays(t *testing.T) {
	api, mux, err := newAPI(Config{UDPPort: 0})
	if err != nil {
		t.Fatalf("newAPI: %v", err)
	}
	t.Cleanup(func() { _ = mux.Close() })

	serverPC, err := api.NewPeerConnection(pion.Configuration{})
	if err != nil {
		t.Fatalf("server NewPeerConnection: %v", err)
	}
	t.Cleanup(func() { _ = serverPC.Close() })

	// The client is a bare pion PeerConnection with default settings --
	// exactly what a real browser peer looks like from this side.
	clientPC, err := pion.NewPeerConnection(pion.Configuration{})
	if err != nil {
		t.Fatalf("client NewPeerConnection: %v", err)
	}
	t.Cleanup(func() { _ = clientPC.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	fromClient := make(chan string, 1)
	fromServer := make(chan string, 1)
	serverOpen := make(chan *pion.DataChannel, 1)

	serverPC.OnDataChannel(func(dc *pion.DataChannel) {
		dc.OnOpen(func() { serverOpen <- dc })
		dc.OnMessage(func(msg pion.DataChannelMessage) { fromClient <- string(msg.Data) })
	})

	clientChannel, err := clientPC.CreateDataChannel("game", nil)
	if err != nil {
		t.Fatalf("CreateDataChannel: %v", err)
	}

	clientOpen := make(chan struct{})
	clientChannel.OnOpen(func() { close(clientOpen) })
	clientChannel.OnMessage(func(msg pion.DataChannelMessage) { fromServer <- string(msg.Data) })

	offer := gatherLocalOffer(t, ctx, clientPC)

	answer, err := answerOffer(ctx, serverPC, offer)
	if err != nil {
		t.Fatalf("answerOffer: %v", err)
	}

	if err := clientPC.SetRemoteDescription(*answer); err != nil {
		t.Fatalf("client SetRemoteDescription: %v", err)
	}

	select {
	case <-clientOpen:
	case <-ctx.Done():
		t.Fatal("client data channel never opened")
	}

	var serverChannel *pion.DataChannel

	select {
	case serverChannel = <-serverOpen:
	case <-ctx.Done():
		t.Fatal("server data channel never opened")
	}

	if err := clientChannel.SendText("hello from client"); err != nil {
		t.Fatalf("client SendText: %v", err)
	}

	select {
	case got := <-fromClient:
		if got != "hello from client" {
			t.Fatalf("server received %q, want %q", got, "hello from client")
		}
	case <-ctx.Done():
		t.Fatal("server never received the client's message")
	}

	if err := serverChannel.SendText("hello from server"); err != nil {
		t.Fatalf("server SendText: %v", err)
	}

	select {
	case got := <-fromServer:
		if got != "hello from server" {
			t.Fatalf("client received %q, want %q", got, "hello from server")
		}
	case <-ctx.Done():
		t.Fatal("client never received the server's message")
	}
}
