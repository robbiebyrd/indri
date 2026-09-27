// Package webrtc implements transport.Transport over WebRTC data channels,
// using pion.
//
// Signaling is one HTTP exchange with complete (non-trickle) ICE:
//
//	POST /webrtc/offer   body: the client's offer as {"type":"offer","sdp":…},
//	                     created after the client has added its data channel
//	                     and finished gathering candidates. Responds with the
//	                     answer in the same shape, candidates included.
//
// Messages then flow over the client's data channel: strings are text,
// binary messages are binary. The channel must be ordered (the default).
package webrtc

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	pion "github.com/pion/webrtc/v4"

	"github.com/robbiebyrd/indri/internal/transport"
)

const (
	offerPath = "/webrtc/offer"

	// maxOfferSize bounds the signaling body. Complete offers carry every
	// gathered candidate, so they are larger than a game message.
	maxOfferSize = 64 << 10

	// flushTimeout bounds how long a closing peer waits for queued data to
	// leave before the connection is torn down.
	flushTimeout = time.Second
)

// Config tunes the transport.
type Config struct {
	// AllowedOrigins is the comma-separated browser Origin allowlist.
	AllowedOrigins string
	// BufferSize is how many outbound messages may queue per connection.
	BufferSize int
	// ICEServers are STUN/TURN URLs. None are needed when the server has a
	// public IP.
	ICEServers []string
	// MaxPeers caps concurrent peer connections, pending or open. Each holds
	// UDP sockets and DTLS state, and the offer endpoint is unauthenticated.
	MaxPeers int
	// NAT1To1IPs are public IPs to advertise instead of the host's own, for a
	// server behind 1:1 NAT (e.g. in Docker or a cloud VM).
	NAT1To1IPs []string
	// UDPPortMin and UDPPortMax restrict ICE to a port range; zero means any.
	UDPPortMin, UDPPortMax uint16
	// GatherTimeout bounds ICE gathering while answering an offer. Keep it
	// under the HTTP server's WriteTimeout.
	GatherTimeout time.Duration
	// OpenTimeout is how long a peer may take to open its data channel after
	// the answer before it is closed.
	OpenTimeout time.Duration
	// IncludeLoopback offers loopback candidates, for a client and server on
	// the same host.
	IncludeLoopback bool
}

// Transport is a WebRTC data channel transport.
type Transport struct {
	*transport.Hub
	cfg Config
	api *pion.API

	peersMu sync.Mutex
	peers   map[*peer]struct{}
}

func New(cfg Config) (*Transport, error) {
	var se pion.SettingEngine

	if cfg.UDPPortMin != 0 || cfg.UDPPortMax != 0 {
		if err := se.SetEphemeralUDPPortRange(cfg.UDPPortMin, cfg.UDPPortMax); err != nil {
			return nil, fmt.Errorf("webrtc udp port range: %w", err)
		}
	}

	if len(cfg.NAT1To1IPs) > 0 {
		err := se.SetICEAddressRewriteRules(pion.ICEAddressRewriteRule{
			External:        cfg.NAT1To1IPs,
			AsCandidateType: pion.ICECandidateTypeHost,
		})
		if err != nil {
			return nil, fmt.Errorf("webrtc nat 1:1 ips: %w", err)
		}
	}

	se.SetIncludeLoopbackCandidate(cfg.IncludeLoopback)

	return &Transport{
		Hub:   transport.NewHub(),
		cfg:   cfg,
		api:   pion.NewAPI(pion.WithSettingEngine(se)),
		peers: make(map[*peer]struct{}),
	}, nil
}

func (t *Transport) Register(mux *http.ServeMux) {
	transport.Route(mux, t.cfg.AllowedOrigins, http.MethodPost, offerPath, http.HandlerFunc(t.offer))
}

// Close refuses new peers and closes every peer, including those whose data
// channel has not opened yet and so are not in the hub.
func (t *Transport) Close() error {
	err := t.Hub.Close()

	t.peersMu.Lock()
	peers := make([]*peer, 0, len(t.peers))
	for p := range t.peers {
		peers = append(peers, p)
	}
	t.peersMu.Unlock()

	for _, p := range peers {
		p.close()
	}

	return err
}

func (t *Transport) offer(w http.ResponseWriter, r *http.Request) {
	var offer pion.SessionDescription

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxOfferSize)).Decode(&offer); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			http.Error(w, "offer too large", http.StatusRequestEntityTooLarge)
			return
		}

		http.Error(w, "invalid offer", http.StatusBadRequest)

		return
	}

	if offer.Type != pion.SDPTypeOffer {
		http.Error(w, "expected an offer", http.StatusBadRequest)
		return
	}

	p, status, err := t.newPeer()
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}

	answer, status, err := p.answer(offer)
	if err != nil {
		p.close()
		http.Error(w, err.Error(), status)

		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(answer)
}

// newPeer creates a peer connection if the cap allows, returning an HTTP
// status with any error.
func (t *Transport) newPeer() (*peer, int, error) {
	if t.IsClosed() {
		return nil, http.StatusServiceUnavailable, errors.New("server shutting down")
	}

	id, err := transport.NewConnectionID()
	if err != nil {
		return nil, http.StatusInternalServerError, errors.New("could not create peer")
	}

	var ice []pion.ICEServer
	if len(t.cfg.ICEServers) > 0 {
		ice = []pion.ICEServer{{URLs: t.cfg.ICEServers}}
	}

	p := &peer{t: t, id: id}

	t.peersMu.Lock()
	if len(t.peers) >= t.cfg.MaxPeers {
		t.peersMu.Unlock()
		return nil, http.StatusServiceUnavailable, errors.New("too many peers")
	}
	t.peers[p] = struct{}{}
	t.peersMu.Unlock()

	pc, err := t.api.NewPeerConnection(pion.Configuration{ICEServers: ice})
	if err != nil {
		t.release(p)
		return nil, http.StatusInternalServerError, errors.New("could not create peer")
	}

	p.pc = pc

	pc.OnDataChannel(p.attach)
	pc.OnConnectionStateChange(func(s pion.PeerConnectionState) {
		// Disconnected can recover; Failed and Closed cannot. This also
		// catches peers that vanish without closing their data channel.
		if s == pion.PeerConnectionStateFailed || s == pion.PeerConnectionStateClosed {
			p.close()
		}
	})

	return p, 0, nil
}

func (t *Transport) release(p *peer) {
	t.peersMu.Lock()
	defer t.peersMu.Unlock()

	delete(t.peers, p)
}

// peer is one client's peer connection and, once its data channel opens,
// its Indri connection.
type peer struct {
	t  *Transport
	id string
	pc *pion.PeerConnection

	mu     sync.Mutex
	conn   *transport.QueuedConn
	opened atomic.Bool

	closeOnce sync.Once
}

func (p *peer) answer(offer pion.SessionDescription) (*pion.SessionDescription, int, error) {
	if err := p.pc.SetRemoteDescription(offer); err != nil {
		return nil, http.StatusBadRequest, errors.New("invalid offer")
	}

	answer, err := p.pc.CreateAnswer(nil)
	if err != nil {
		return nil, http.StatusBadRequest, errors.New("could not answer offer")
	}

	gathered := pion.GatheringCompletePromise(p.pc)

	if err := p.pc.SetLocalDescription(answer); err != nil {
		return nil, http.StatusInternalServerError, errors.New("could not answer offer")
	}

	select {
	case <-gathered:
	case <-time.After(p.t.cfg.GatherTimeout):
		return nil, http.StatusGatewayTimeout, errors.New("ice gathering timed out")
	}

	time.AfterFunc(p.t.cfg.OpenTimeout, func() {
		if !p.opened.Load() {
			p.close()
		}
	})

	return p.pc.LocalDescription(), 0, nil
}

// attach adopts the client's first data channel; any others are refused.
func (p *peer) attach(dc *pion.DataChannel) {
	p.mu.Lock()
	if p.conn != nil {
		p.mu.Unlock()
		_ = dc.Close()

		return
	}

	conn := transport.NewQueuedConn(p.t.cfg.BufferSize, nil)
	p.conn = conn
	p.mu.Unlock()

	handlers := p.t.Handlers()

	dc.OnOpen(func() {
		p.opened.Store(true)

		if err := p.t.Hub.Add(p.id, conn); err != nil {
			p.close()
			return
		}

		go p.writer(dc, conn)

		conn.Connected(handlers)
	})

	dc.OnMessage(func(m pion.DataChannelMessage) {
		conn.Deliver(handlers, m.Data)
	})

	dc.OnClose(p.close)
}

// writer is the only goroutine that sends on the data channel.
func (p *peer) writer(dc *pion.DataChannel, conn *transport.QueuedConn) {
	for {
		select {
		case f := <-conn.Outbound():
			if send(dc, f) != nil {
				p.close()
				return
			}
		case <-conn.Done():
			// A server-side close (kick, shutdown): flush what was queued
			// and let it leave before tearing the connection down.
			conn.Drain(func(f transport.Frame) bool { return send(dc, f) == nil })

			deadline := time.Now().Add(flushTimeout)
			for dc.BufferedAmount() > 0 && time.Now().Before(deadline) {
				time.Sleep(10 * time.Millisecond)
			}

			p.close()

			return
		}
	}
}

func send(dc *pion.DataChannel, f transport.Frame) error {
	if f.Binary {
		return dc.Send(f.Data)
	}

	return dc.SendText(string(f.Data))
}

// close tears the peer down once: it leaves the hub, fires Disconnect,
// frees its slot, and closes the peer connection.
func (p *peer) close() {
	p.closeOnce.Do(func() {
		p.t.Hub.Remove(p.id)

		p.mu.Lock()
		conn := p.conn
		p.mu.Unlock()

		if conn != nil {
			_ = conn.Close()
			conn.Disconnected(p.t.Handlers())
		}

		p.t.release(p)

		// Closing from inside a pion callback must not block that callback.
		if p.pc != nil {
			go func() { _ = p.pc.Close() }()
		}
	})
}
