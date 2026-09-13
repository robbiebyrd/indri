package webrtc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	pion "github.com/pion/webrtc/v4"

	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/transport"
)

// signalPath is the HTTP route a client POSTs its SDP offer to.
const signalPath = "/rtc/offer"

// gameChannel carries client actions and is piped into Handlers.Message,
// exactly like any other transport's inbound messages. signalChannel is
// reserved for renegotiation (story 041); messages on it must never reach the
// action router.
const (
	gameChannel   = "game"
	signalChannel = "signal"
)

// defaultMaxPeers and defaultPendingTTL are the production defaults. Tests
// override MaxPeers/PendingTTL with small/short values rather than sleeping
// for these.
const (
	defaultMaxPeers   = 1000
	defaultPendingTTL = 30 * time.Second
)

// answerTimeout bounds the non-trickle ICE gathering wait inside answerOffer,
// so a misconfigured NAT1To1IPs (or a peer closing mid-gather) fails the
// signalling request instead of hanging it.
const answerTimeout = 5 * time.Second

// SessionLookup resolves a session from its bearer token (the same opaque
// token login and reconnect issue). Satisfied by the session store; mirrors
// the identical interface in internal/transport/sse and internal/transport/rest.
type SessionLookup interface {
	GetByToken(token string) (*models.Session, error)
}

// Transport is the WebRTC transport: one HTTP route performs non-trickle SDP
// signalling, and every resulting PeerConnection's "game" DataChannel becomes
// a push+pull transport.Conn. The set of open conns and the fan-out over them
// come from the embedded Registry, shared with sse/graphql/rest.
type Transport struct {
	*transport.Registry

	sessions SessionLookup
	api      *pion.API
	udpMux   io.Closer

	handlers transport.Handlers

	// MaxPeers caps concurrent PeerConnections (pending or connected) this
	// transport will allocate. Anonymous signalling is a resource-exhaustion
	// vector -- each POST would otherwise allocate an ICE agent and a mux
	// registration -- so once the cap is reached, offer returns 503 rather
	// than allocating. Set before Register; zero uses defaultMaxPeers.
	MaxPeers int

	// PendingTTL bounds how long a peer may sit without reaching Connected
	// before it is closed and dropped. Set before Register; zero uses
	// defaultPendingTTL.
	PendingTTL time.Duration

	mu     sync.Mutex
	peers  map[string]*peer // keyed by a server-generated peer id, not session
	nextID uint64
}

var _ transport.Transport = (*Transport)(nil)

// New builds the WebRTC transport. sessions authenticates bearer tokens at
// signalling time; cfg configures the shared pion API and UDP mux built once
// here and reused for every peer.
func New(sessions SessionLookup, cfg Config) (*Transport, error) {
	if sessions == nil {
		return nil, errors.New("webrtc: sessions is required")
	}

	api, mux, err := newAPI(cfg)
	if err != nil {
		return nil, fmt.Errorf("webrtc: building api: %w", err)
	}

	return &Transport{
		Registry: &transport.Registry{},
		sessions: sessions,
		api:      api,
		udpMux:   mux,
		peers:    make(map[string]*peer),
	}, nil
}

// Handle stores the lifecycle callbacks. Call before Register, matching the
// transport.Transport contract.
func (t *Transport) Handle(h transport.Handlers) {
	t.handlers = h
}

// Register mounts the signalling route.
func (t *Transport) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST "+signalPath, t.offer)
}

// Close stops accepting connections, closes every open conn (via the embedded
// Registry) and releases the shared UDP mux.
func (t *Transport) Close() error {
	if err := t.udpMux.Close(); err != nil {
		log.Printf("webrtc: closing udp mux: %v", err)
	}

	return t.Registry.Close()
}

func (t *Transport) maxPeers() int {
	if t.MaxPeers <= 0 {
		return defaultMaxPeers
	}

	return t.MaxPeers
}

func (t *Transport) pendingTTL() time.Duration {
	if t.PendingTTL <= 0 {
		return defaultPendingTTL
	}

	return t.PendingTTL
}

// reserve claims a slot in the peer table before any PeerConnection is
// allocated, so two concurrent offers cannot both pass a check-then-act race
// and exceed the cap: the placeholder counts toward maxPeers immediately.
func (t *Transport) reserve() (id string, ok bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if len(t.peers) >= t.maxPeers() {
		return "", false
	}

	t.nextID++
	id = strconv.FormatUint(t.nextID, 10)
	t.peers[id] = nil

	return id, true
}

func (t *Transport) store(id string, p *peer) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.peers[id] = p
}

func (t *Transport) deregister(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	delete(t.peers, id)
}

// peerCount reports how many peers currently occupy a slot, reserved or not.
// Used by the cap check above and by tests asserting on the pending-TTL and
// cap behaviour.
func (t *Transport) peerCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()

	return len(t.peers)
}

// offer handles the non-trickle SDP exchange: decode the client's offer,
// allocate a PeerConnection (subject to the pending-peer TTL and global cap),
// and return one complete SDP answer whose ICE candidates are already
// gathered.
func (t *Transport) offer(w http.ResponseWriter, r *http.Request) {
	signal, err := DecodeSignal(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)

		return
	}

	if signal.Type != "offer" {
		http.Error(w, fmt.Sprintf("expected an offer, got %q", signal.Type), http.StatusBadRequest)

		return
	}

	id, ok := t.reserve()
	if !ok {
		// Anonymous signalling is a resource-exhaustion vector: refuse before
		// ever allocating a PeerConnection, ICE agent, or mux registration.
		http.Error(w, "too many peer connections", http.StatusServiceUnavailable)

		return
	}

	sessionID := t.authenticate(r)

	pc, err := t.api.NewPeerConnection(pion.Configuration{})
	if err != nil {
		t.deregister(id)
		log.Printf("webrtc: creating peer connection: %v", err)
		http.Error(w, "creating peer connection", http.StatusInternalServerError)

		return
	}

	p := newPeer(pc, nil, func() { t.deregister(id) })
	t.store(id, p)

	pc.OnDataChannel(func(dc *pion.DataChannel) {
		switch dc.Label() {
		case gameChannel:
			t.attachGame(p, sessionID, dc)
		case signalChannel:
			t.attachSignal(p, dc)
		default:
			log.Printf("webrtc: peer %s opened data channel with unrecognised label %q", id, dc.Label())
		}
	})

	ctx, cancel := context.WithTimeout(r.Context(), answerTimeout)
	defer cancel()

	answer, err := answerOffer(ctx, pc, pion.SessionDescription{Type: pion.SDPTypeOffer, SDP: signal.SDP})
	if err != nil {
		p.teardown.Do(p.close)
		log.Printf("webrtc: answering offer: %v", err)
		http.Error(w, "answering offer", http.StatusInternalServerError)

		return
	}

	t.schedulePendingTTL(p)

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(Signal{Type: "answer", SDP: answer.SDP}); err != nil {
		log.Printf("webrtc: writing answer: %v", err)
	}
}

// schedulePendingTTL closes and drops p if it never reaches Connected within
// the configured TTL. A peer that already tore down for another reason (e.g.
// Failed) is a safe no-op here: teardown is guarded by sync.Once.
func (t *Transport) schedulePendingTTL(p *peer) {
	time.AfterFunc(t.pendingTTL(), func() {
		if p.pc.ConnectionState() != pion.PeerConnectionStateConnected {
			p.teardown.Do(p.close)
		}
	})
}

// attachGame wires the client's "game" DataChannel into a transport.Conn:
// opening registers it for fan-out and fires Handlers.Connect; messages flow
// into Handlers.Message exactly like any other transport, which is what lets
// login bind a session with no new action code; closing deregisters it and
// fires Handlers.Disconnect.
func (t *Transport) attachGame(p *peer, sessionID string, dc *pion.DataChannel) {
	conn := newConn(sessionID, dc, negotiatedMaxMessageSize(p.pc))
	p.setConn(conn)

	// Make it an invariant that closing this conn tears down its peer, so
	// every path that closes a conn (kick via Registry.Disconnect, a natural
	// DataChannel close, transport shutdown) reclaims the PeerConnection too
	// -- not just kick. Set once, here, before conn is reachable from any
	// other goroutine (AddSink/OnClose are registered below).
	conn.onTeardown = func() { p.teardown.Do(p.close) }

	dc.OnOpen(func() {
		t.AddSink(conn)

		if t.handlers.Connect != nil {
			t.handlers.Connect(conn)
		}
	})

	dc.OnClose(func() {
		t.RemoveSink(conn)
		_ = conn.Close()

		if t.handlers.Disconnect != nil {
			t.handlers.Disconnect(conn)
		}
	})

	dc.OnMessage(func(msg pion.DataChannelMessage) {
		// A kick closes the conn, but pion's DataChannel stays open until its
		// own teardown finishes. Without this guard a kicked player keeps
		// reaching the action router: they stop receiving, yet carry on
		// sending. Closing the conn has to mean both directions.
		if conn.IsClosed() {
			return
		}

		if t.handlers.Message != nil {
			t.handlers.Message(conn, msg.Data)
		}
	})
}

// negotiatedMaxMessageSize reads the SCTP max-message-size pion negotiated
// for pc's association (criterion 2) -- the same negotiation the browser's
// own RTCSctpTransport.maxMessageSize reports for the same peer. SCTP()
// never returns nil for a PeerConnection built through New/newAPI, and
// GetCapabilities is itself nil-safe on its receiver, but the zero value this
// returns before any association exists is exactly newConn's signal to fall
// back to fallbackMaxMessageSize -- so no extra nil check earns its keep
// here.
func negotiatedMaxMessageSize(pc *pion.PeerConnection) uint32 {
	return pc.SCTP().GetCapabilities().MaxMessageSize
}

// attachSignal wires the "signal" DataChannel to p: renegotiate (story 041,
// renegotiate.go) sends its offers here and awaits the matching answer.
// Deliberately not connected to Handlers.Message -- nothing this channel
// carries may ever reach the action router.
func (t *Transport) attachSignal(p *peer, dc *pion.DataChannel) {
	p.setSignal(dc)

	dc.OnMessage(func(msg pion.DataChannelMessage) {
		p.handleSignalMessage(msg.Data)
	})
}

// authenticate resolves the caller's bearer token to a session id. An empty
// or unrecognised token yields "", matching transport.NewKeys: the resulting
// conn carries no session key until a login binds one.
func (t *Transport) authenticate(r *http.Request) string {
	session, err := t.sessions.GetByToken(bearerToken(r))
	if err != nil || session == nil {
		return ""
	}

	return session.ID.Hex()
}

// bearerToken reads the session token from the Authorization header.
func bearerToken(r *http.Request) string {
	return strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
}
