package webrtc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	pion "github.com/pion/webrtc/v4"

	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/transport"
)

// noSessions never authenticates anyone, standing in for an unauthenticated
// deployment or an invalid/missing token.
type noSessions struct{}

func (noSessions) GetByToken(string) (*models.Session, error) {
	return nil, errors.New("no such session")
}

// fakeSessions resolves exactly one token, mirroring sse_test.go's double.
type fakeSessions struct {
	token string
	id    bson.ObjectID
}

func (f fakeSessions) GetByToken(token string) (*models.Session, error) {
	if token == "" || token != f.token {
		return nil, errors.New("no such session")
	}

	return &models.Session{ID: f.id, Token: token}, nil
}

// fakeTransport is a Registry promoted to a full Transport, standing in for
// another protocol's transport inside a Multi -- mirrors
// internal/transport/registry_test.go's own fakeTransport.
type fakeTransport struct{ *transport.Registry }

func (fakeTransport) Handle(transport.Handlers) {}
func (fakeTransport) Register(*http.ServeMux)   {}

func newTestTransport(t *testing.T, sessions SessionLookup) *Transport {
	t.Helper()

	tr, err := New(sessions, Config{UDPPort: 0})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	t.Cleanup(func() { _ = tr.Close() })

	return tr
}

func newTestServer(t *testing.T, tr *Transport) string {
	t.Helper()

	mux := http.NewServeMux()
	tr.Register(mux)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return srv.URL + signalPath
}

// clientHandshake drives an in-process pion client through a full offer/answer
// exchange against url: it creates one data channel per label, POSTs the
// gathered offer with the given bearer token, applies the returned answer,
// and waits for every channel to open.
func clientHandshake(t *testing.T, url, bearer string, labels ...string) (*pion.PeerConnection, map[string]*pion.DataChannel) {
	t.Helper()

	clientPC, err := pion.NewPeerConnection(pion.Configuration{})
	if err != nil {
		t.Fatalf("client NewPeerConnection: %v", err)
	}

	t.Cleanup(func() { _ = clientPC.Close() })

	channels := make(map[string]*pion.DataChannel, len(labels))
	opened := make(chan string, len(labels))

	for _, label := range labels {
		dc, err := clientPC.CreateDataChannel(label, nil)
		if err != nil {
			t.Fatalf("CreateDataChannel(%q): %v", label, err)
		}

		label := label
		dc.OnOpen(func() { opened <- label })
		channels[label] = dc
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	offer := gatherLocalOffer(t, ctx, clientPC)

	body, err := json.Marshal(Signal{Type: "offer", SDP: offer.SDP})
	if err != nil {
		t.Fatalf("marshal offer: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST offer: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST offer status = %d, want 200", resp.StatusCode)
	}

	var answer Signal
	if err := json.NewDecoder(resp.Body).Decode(&answer); err != nil {
		t.Fatalf("decode answer: %v", err)
	}

	if err := clientPC.SetRemoteDescription(pion.SessionDescription{Type: pion.SDPTypeAnswer, SDP: answer.SDP}); err != nil {
		t.Fatalf("client SetRemoteDescription: %v", err)
	}

	for range labels {
		select {
		case <-opened:
		case <-ctx.Done():
			t.Fatal("data channel(s) did not open in time")
		}
	}

	return clientPC, channels
}

// Criterion 1: POST to the signalling route with a pion-generated offer
// returns an answer and registers a conn.
func TestTransport_OfferReturnsAnswerAndRegistersConn(t *testing.T) {
	tr := newTestTransport(t, noSessions{})

	connected := make(chan transport.Conn, 1)
	tr.Handle(transport.Handlers{Connect: func(c transport.Conn) { connected <- c }})

	url := newTestServer(t, tr)

	clientHandshake(t, url, "", gameChannel)

	select {
	case <-connected:
	case <-time.After(testTimeout):
		t.Fatal("Connect handler was never invoked; conn was not registered")
	}

	conns, err := tr.Conns()
	if err != nil {
		t.Fatalf("Conns: %v", err)
	}

	if len(conns) != 1 {
		t.Fatalf("Conns() = %d, want 1", len(conns))
	}
}

// Criterion 2: a request carrying a valid bearer token binds SessionIDKey at
// handshake so BroadcastFilter reaches it.
func TestTransport_BearerTokenBindsSessionKey(t *testing.T) {
	sessionID := bson.NewObjectID()
	sessions := fakeSessions{token: "tok-123", id: sessionID}
	tr := newTestTransport(t, sessions)

	connected := make(chan transport.Conn, 1)
	tr.Handle(transport.Handlers{Connect: func(c transport.Conn) { connected <- c }})

	url := newTestServer(t, tr)

	_, channels := clientHandshake(t, url, "tok-123", gameChannel)

	var conn transport.Conn

	select {
	case conn = <-connected:
	case <-time.After(testTimeout):
		t.Fatal("Connect handler was never invoked")
	}

	got, ok := conn.Get(transport.SessionIDKey)
	if !ok {
		t.Fatal("authenticated conn carries no session key")
	}

	if got != sessionID.Hex() {
		t.Fatalf("session key = %v, want %s", got, sessionID.Hex())
	}

	received := make(chan string, 1)
	channels[gameChannel].OnMessage(func(msg pion.DataChannelMessage) { received <- string(msg.Data) })

	err := tr.BroadcastFilter([]byte("hello"), func(c transport.Conn) bool {
		id, _ := c.Get(transport.SessionIDKey)

		return id == sessionID.Hex()
	})
	if err != nil {
		t.Fatalf("BroadcastFilter: %v", err)
	}

	select {
	case got := <-received:
		if got != "hello" {
			t.Fatalf("received %q, want %q", got, "hello")
		}
	case <-time.After(testTimeout):
		t.Fatal("BroadcastFilter did not reach the session-bound conn")
	}
}

// Criterion 3: a request with no token yields an anonymous conn carrying no
// session key.
func TestTransport_NoTokenYieldsAnonymousConn(t *testing.T) {
	tr := newTestTransport(t, noSessions{})

	connected := make(chan transport.Conn, 1)
	tr.Handle(transport.Handlers{Connect: func(c transport.Conn) { connected <- c }})

	url := newTestServer(t, tr)

	clientHandshake(t, url, "", gameChannel)

	var conn transport.Conn

	select {
	case conn = <-connected:
	case <-time.After(testTimeout):
		t.Fatal("Connect handler was never invoked")
	}

	if _, ok := conn.Get(transport.SessionIDKey); ok {
		t.Fatal("anonymous conn carries a session key")
	}
}

// Criterion 4: a login sent over the DataChannel binds the session through
// the existing dispatch path, with no new action code. This proves the exact
// wiring boot.handleClientMessage relies on -- a message reaches
// Handlers.Message with this conn, and Set(SessionIDKey, ...) on it then
// makes BroadcastFilter reach it -- without pulling in boot or the router.
func TestTransport_GameChannelMessageBindsSessionThroughDispatchSeam(t *testing.T) {
	tr := newTestTransport(t, noSessions{})

	type inbound struct {
		conn transport.Conn
		msg  []byte
	}

	messages := make(chan inbound, 1)

	tr.Handle(transport.Handlers{Message: func(c transport.Conn, msg []byte) {
		messages <- inbound{c, msg}
	}})

	url := newTestServer(t, tr)

	_, channels := clientHandshake(t, url, "", gameChannel)

	if err := channels[gameChannel].SendText(`{"action":"login"}`); err != nil {
		t.Fatalf("SendText: %v", err)
	}

	var got inbound

	select {
	case got = <-messages:
	case <-time.After(testTimeout):
		t.Fatal("game channel message never reached Handlers.Message")
	}

	if string(got.msg) != `{"action":"login"}` {
		t.Fatalf("message = %q, want the login payload", got.msg)
	}

	// This is exactly what boot.handleClientMessage does with a dispatch
	// result carrying a bound session.
	got.conn.Set(transport.SessionIDKey, "session-abc")

	received := make(chan string, 1)
	channels[gameChannel].OnMessage(func(msg pion.DataChannelMessage) { received <- string(msg.Data) })

	err := tr.BroadcastFilter([]byte("welcome"), func(c transport.Conn) bool {
		id, _ := c.Get(transport.SessionIDKey)

		return id == "session-abc"
	})
	if err != nil {
		t.Fatalf("BroadcastFilter: %v", err)
	}

	select {
	case got := <-received:
		if got != "welcome" {
			t.Fatalf("received %q, want %q", got, "welcome")
		}
	case <-time.After(testTimeout):
		t.Fatal("BroadcastFilter did not reach the newly-bound session")
	}
}

// Criterion 5: a peer that never reaches connected is closed and dropped
// after the pending TTL. The client deliberately never applies the answer and
// closes immediately, so the server's PeerConnection can never complete DTLS.
func TestTransport_PendingPeerTTLClosesUnconnectedPeer(t *testing.T) {
	tr := newTestTransport(t, noSessions{})
	tr.PendingTTL = 50 * time.Millisecond

	url := newTestServer(t, tr)

	clientPC, err := pion.NewPeerConnection(pion.Configuration{})
	if err != nil {
		t.Fatalf("client NewPeerConnection: %v", err)
	}

	if _, err := clientPC.CreateDataChannel(gameChannel, nil); err != nil {
		t.Fatalf("CreateDataChannel: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	offer := gatherLocalOffer(t, ctx, clientPC)

	body, err := json.Marshal(Signal{Type: "offer", SDP: offer.SDP})
	if err != nil {
		t.Fatalf("marshal offer: %v", err)
	}

	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST offer: %v", err)
	}

	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	if err := clientPC.Close(); err != nil {
		t.Fatalf("closing client pc: %v", err)
	}

	deadline := time.Now().Add(testTimeout)

	for time.Now().Before(deadline) {
		if tr.peerCount() == 0 {
			return
		}

		time.Sleep(5 * time.Millisecond)
	}

	t.Fatalf("peer was not dropped after the pending TTL; peerCount = %d", tr.peerCount())
}

// Criterion 6: exceeding the global peer cap returns 503 rather than
// allocating.
func TestTransport_ExceedingMaxPeersReturns503WithoutAllocating(t *testing.T) {
	tr := newTestTransport(t, noSessions{})
	tr.MaxPeers = 1
	tr.PendingTTL = testTimeout

	url := newTestServer(t, tr)

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	clientPC1, err := pion.NewPeerConnection(pion.Configuration{})
	if err != nil {
		t.Fatalf("client1 NewPeerConnection: %v", err)
	}

	t.Cleanup(func() { _ = clientPC1.Close() })

	if _, err := clientPC1.CreateDataChannel(gameChannel, nil); err != nil {
		t.Fatalf("CreateDataChannel: %v", err)
	}

	offer1 := gatherLocalOffer(t, ctx, clientPC1)

	body1, err := json.Marshal(Signal{Type: "offer", SDP: offer1.SDP})
	if err != nil {
		t.Fatalf("marshal offer1: %v", err)
	}

	resp1, err := http.Post(url, "application/json", bytes.NewReader(body1))
	if err != nil {
		t.Fatalf("POST offer1: %v", err)
	}

	resp1.Body.Close()

	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("first offer status = %d, want 200", resp1.StatusCode)
	}

	if got := tr.peerCount(); got != 1 {
		t.Fatalf("peerCount after first offer = %d, want 1", got)
	}

	clientPC2, err := pion.NewPeerConnection(pion.Configuration{})
	if err != nil {
		t.Fatalf("client2 NewPeerConnection: %v", err)
	}

	t.Cleanup(func() { _ = clientPC2.Close() })

	if _, err := clientPC2.CreateDataChannel(gameChannel, nil); err != nil {
		t.Fatalf("CreateDataChannel: %v", err)
	}

	offer2 := gatherLocalOffer(t, ctx, clientPC2)

	body2, err := json.Marshal(Signal{Type: "offer", SDP: offer2.SDP})
	if err != nil {
		t.Fatalf("marshal offer2: %v", err)
	}

	resp2, err := http.Post(url, "application/json", bytes.NewReader(body2))
	if err != nil {
		t.Fatalf("POST offer2: %v", err)
	}

	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("second offer status = %d, want 503", resp2.StatusCode)
	}

	if got := tr.peerCount(); got != 1 {
		t.Fatalf("peerCount after rejected offer = %d, want still 1 (no allocation)", got)
	}
}

// Criterion 7: Disconnect closes a conn held on another transport, reached
// through the aggregate -- mirrors
// transport.TestRegistry_DisconnectReachesPeerTransport, but proves it for
// this Transport specifically (i.e. embedding Registry was not broken).
func TestTransport_DisconnectReachesPeerAcrossAggregate(t *testing.T) {
	local := newTestTransport(t, noSessions{})
	remote := fakeTransport{&transport.Registry{}}

	onPeer := transport.NewBufferedConn[[]byte]("kick-me", 1)
	remote.AddSink(onPeer)

	local.SetPeer(transport.NewMulti(local, remote))
	local.Disconnect([]string{"kick-me"})

	if !onPeer.IsClosed() {
		t.Fatal("connection on the peer transport was not disconnected")
	}
}

// Criterion 8: the game and signal DataChannels are routed by label.
func TestTransport_DataChannelsRoutedByLabel(t *testing.T) {
	tr := newTestTransport(t, noSessions{})

	var calls int32

	received := make(chan []byte, 4)

	tr.Handle(transport.Handlers{Message: func(_ transport.Conn, msg []byte) {
		atomic.AddInt32(&calls, 1)
		received <- msg
	}})

	url := newTestServer(t, tr)

	_, channels := clientHandshake(t, url, "", gameChannel, signalChannel)

	if err := channels[signalChannel].SendText("signal-payload"); err != nil {
		t.Fatalf("SendText(signal): %v", err)
	}

	if err := channels[gameChannel].SendText("game-payload"); err != nil {
		t.Fatalf("SendText(game): %v", err)
	}

	select {
	case got := <-received:
		if string(got) != "game-payload" {
			t.Fatalf("Handlers.Message received %q, want %q (routing by label failed)", got, "game-payload")
		}
	case <-time.After(testTimeout):
		t.Fatal("game channel message never reached Handlers.Message")
	}

	// Grace period for a wrongly-wired signal message to arrive, which would
	// show up as a second call.
	time.Sleep(50 * time.Millisecond)

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("Handlers.Message called %d times, want exactly 1 -- the signal channel must not route to it", got)
	}
}

// Criterion 9: messages on the signal channel never reach the action router.
func TestTransport_SignalChannelMessagesNeverReachActionRouter(t *testing.T) {
	tr := newTestTransport(t, noSessions{})

	var calls int32

	tr.Handle(transport.Handlers{Message: func(transport.Conn, []byte) {
		atomic.AddInt32(&calls, 1)
	}})

	url := newTestServer(t, tr)

	_, channels := clientHandshake(t, url, "", gameChannel, signalChannel)

	if err := channels[signalChannel].SendText(`{"type":"offer","sdp":"..."}`); err != nil {
		t.Fatalf("SendText(signal): %v", err)
	}

	// There is no positive event to wait on for a message that must NOT
	// arrive, so this bounds how long a wrongly-wired signal channel gets to
	// (incorrectly) reach the handler before we conclude it didn't.
	time.Sleep(200 * time.Millisecond)

	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Fatalf("Handlers.Message called %d times from the signal channel, want 0", got)
	}
}

// Kicking a player must stop them sending, not merely stop them receiving.
// Registry.Disconnect closes the conn, but pion's DataChannel stays open until
// its own teardown, so without a guard a kicked player keeps reaching the
// action router. kick is this repo's reference implementation for
// authorisation, so a transport that lets it through is a security defect.
func TestTransport_KickedPeerCannotStillSend(t *testing.T) {
	id := bson.NewObjectID()

	tr := newTestTransport(t, fakeSessions{token: "tok", id: id})

	var calls int32

	tr.Handle(transport.Handlers{Message: func(transport.Conn, []byte) {
		atomic.AddInt32(&calls, 1)
	}})

	url := newTestServer(t, tr)

	_, channels := clientHandshake(t, url, "tok", gameChannel)

	// Kick: exactly what a kick or logout Result drives.
	tr.Disconnect([]string{id.Hex()})

	if err := channels[gameChannel].SendText(`{"action":"join","code":"ABCD"}`); err != nil {
		// A closed DataChannel is an equally good outcome -- the message
		// never left the client.
		return
	}

	// No positive event to wait on for a message that must not arrive.
	time.Sleep(200 * time.Millisecond)

	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Fatalf("Handlers.Message called %d times after the peer was kicked, want 0", got)
	}
}

// serverPeerConnection finds the one server-side *pion.PeerConnection tr
// allocated for the handshake this test drove -- tests live in this package,
// so reaching into the unexported peer table is fine, and it's the only way
// to assert on pion's own state rather than our bookkeeping.
func serverPeerConnection(t *testing.T, tr *Transport) *pion.PeerConnection {
	t.Helper()

	tr.mu.Lock()
	defer tr.mu.Unlock()

	for _, p := range tr.peers {
		if p != nil {
			return p.pc
		}
	}

	t.Fatal("no server-side peer found")

	return nil
}

// A kick must reclaim the PeerConnection itself, not just stop traffic on its
// conn: Registry.Disconnect closing the conn leaves the ICE agent, DTLS and
// SCTP resources alive until pion's own Failed callback fires, or until the
// pending-peer TTL reclaims it -- and that TTL only covers peers that never
// reached Connected, so a kicked, fully-connected peer is not covered by it
// at all. This asserts on pion's own SignalingState reaching Closed, not on
// our own bookkeeping, and separately that the peer table drops the entry so
// it stops counting toward MaxPeers.
func TestTransport_KickClosesThePeerConnection(t *testing.T) {
	id := bson.NewObjectID()

	tr := newTestTransport(t, fakeSessions{token: "tok", id: id})

	url := newTestServer(t, tr)

	clientHandshake(t, url, "tok", gameChannel)

	pc := serverPeerConnection(t, tr)

	tr.Disconnect([]string{id.Hex()})

	deadline := time.Now().Add(testTimeout)

	for time.Now().Before(deadline) && pc.SignalingState() != pion.SignalingStateClosed {
		time.Sleep(5 * time.Millisecond)
	}

	if got := pc.SignalingState(); got != pion.SignalingStateClosed {
		t.Fatalf("PeerConnection SignalingState = %v after kick, want Closed", got)
	}

	deadline = time.Now().Add(testTimeout)

	for time.Now().Before(deadline) && tr.peerCount() != 0 {
		time.Sleep(5 * time.Millisecond)
	}

	if got := tr.peerCount(); got != 0 {
		t.Fatalf("peerCount = %d after kick, want 0 -- the peer must stop counting toward MaxPeers", got)
	}
}
