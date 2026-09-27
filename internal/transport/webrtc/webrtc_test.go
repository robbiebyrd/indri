package webrtc_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	pion "github.com/pion/webrtc/v4"

	"github.com/robbiebyrd/indri/internal/transport"
	"github.com/robbiebyrd/indri/internal/transport/transporttest"
	"github.com/robbiebyrd/indri/internal/transport/webrtc"
)

func testConfig() webrtc.Config {
	return webrtc.Config{
		BufferSize:      256,
		MaxPeers:        16,
		GatherTimeout:   transporttest.Timeout,
		OpenTimeout:     transporttest.Timeout,
		IncludeLoopback: true,
	}
}

// client is an in-process pion peer: no ICE servers and loopback IPv4 only,
// so the test never touches the network. Offering every interface makes ICE
// try every pair — about 2s per connection on a host with VM bridges and
// IPv6, and over 15s under -race — versus milliseconds on loopback.
type client struct {
	pc     *pion.PeerConnection
	dc     *pion.DataChannel
	frames chan transporttest.Frame
	ended  chan struct{}
	endOne sync.Once
}

func (c *client) end() { c.endOne.Do(func() { close(c.ended) }) }

func newPeer(t *testing.T) *pion.PeerConnection {
	t.Helper()

	var se pion.SettingEngine
	se.SetIncludeLoopbackCandidate(true)
	se.SetNetworkTypes([]pion.NetworkType{pion.NetworkTypeUDP4})
	se.SetIPFilter(func(ip net.IP) bool { return ip.IsLoopback() })

	pc, err := pion.NewAPI(pion.WithSettingEngine(se)).NewPeerConnection(pion.Configuration{})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = pc.Close() })

	return pc
}

// offer builds a complete (non-trickle) offer with a data channel on pc.
func offer(t *testing.T, pc *pion.PeerConnection) (*pion.DataChannel, []byte) {
	t.Helper()

	dc, err := pc.CreateDataChannel("indri", nil)
	if err != nil {
		t.Fatal(err)
	}

	o, err := pc.CreateOffer(nil)
	if err != nil {
		t.Fatal(err)
	}

	gathered := pion.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(o); err != nil {
		t.Fatal(err)
	}
	<-gathered

	body, err := json.Marshal(pc.LocalDescription())
	if err != nil {
		t.Fatal(err)
	}

	return dc, body
}

func post(t *testing.T, base string, body []byte) *http.Response {
	t.Helper()

	resp, err := http.Post(base+"/webrtc/offer", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { resp.Body.Close() })

	return resp
}

func dial(t *testing.T, base string) transporttest.Client {
	t.Helper()

	pc := newPeer(t)
	dc, body := offer(t, pc)

	c := &client{pc: pc, dc: dc, frames: make(chan transporttest.Frame, 256), ended: make(chan struct{})}

	opened := make(chan struct{})
	dc.OnOpen(func() { close(opened) })
	dc.OnMessage(func(m pion.DataChannelMessage) {
		c.frames <- transporttest.Frame{Data: m.Data, Binary: !m.IsString}
	})
	dc.OnClose(c.end)
	pc.OnConnectionStateChange(func(s pion.PeerConnectionState) {
		if s == pion.PeerConnectionStateClosed || s == pion.PeerConnectionStateFailed {
			c.end()
		}
	})

	resp := post(t, base, body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("offer status %d", resp.StatusCode)
	}

	var answer pion.SessionDescription
	if err := json.NewDecoder(resp.Body).Decode(&answer); err != nil {
		t.Fatal(err)
	}

	if err := pc.SetRemoteDescription(answer); err != nil {
		t.Fatal(err)
	}

	select {
	case <-opened:
	case <-time.After(transporttest.Timeout):
		t.Fatal("data channel never opened")
	}

	return c
}

func (c *client) Send(msg []byte) error { return c.dc.SendText(string(msg)) }

func (c *client) Receive(timeout time.Duration) (transporttest.Frame, error) {
	// Frames already delivered win over the end signal.
	select {
	case f := <-c.frames:
		return f, nil
	default:
	}

	select {
	case f := <-c.frames:
		return f, nil
	case <-c.ended:
		select {
		case f := <-c.frames:
			return f, nil
		default:
			return transporttest.Frame{}, errors.New("peer connection closed")
		}
	case <-time.After(timeout):
		return transporttest.Frame{}, errors.New("timed out")
	}
}

func (c *client) Close() error { return c.pc.Close() }

func newTransport(t *testing.T, cfg webrtc.Config) *webrtc.Transport {
	t.Helper()

	tr, err := webrtc.New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	return tr
}

func TestConformance(t *testing.T) {
	transporttest.Run(t, transporttest.Harness{
		New:  func(t *testing.T) transport.Transport { return newTransport(t, testConfig()) },
		Dial: dial,
	})
}

func server(t *testing.T, cfg webrtc.Config) string {
	t.Helper()

	tr := newTransport(t, cfg)
	tr.Handle(transport.Handlers{})

	mux := http.NewServeMux()
	tr.Register(mux)
	srv := httptest.NewServer(mux)

	t.Cleanup(func() {
		_ = tr.Close()
		srv.Close()
	})

	return srv.URL
}

func TestOffer_RejectsBadInput(t *testing.T) {
	base := server(t, testConfig())

	cases := map[string][]byte{
		"not json":  []byte("nope"),
		"not offer": []byte(`{"type":"answer","sdp":"v=0"}`),
		"bad sdp":   []byte(`{"type":"offer","sdp":"garbage"}`),
	}

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if resp := post(t, base, body); resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status %d, want 400", resp.StatusCode)
			}
		})
	}

	t.Run("oversized", func(t *testing.T) {
		big := append([]byte(`{"type":"offer","sdp":"`), bytes.Repeat([]byte("a"), 1<<20)...)
		if resp := post(t, base, big); resp.StatusCode != http.StatusRequestEntityTooLarge {
			t.Fatalf("status %d, want 413", resp.StatusCode)
		}
	})
}

// TestOffer_AbandonedPeerReleasesItsSlot covers both leak guards: the peer
// cap refuses a second peer, and a peer whose data channel never opens is
// closed after OpenTimeout, freeing the slot.
func TestOffer_AbandonedPeerReleasesItsSlot(t *testing.T) {
	cfg := testConfig()
	cfg.MaxPeers = 1
	cfg.OpenTimeout = 300 * time.Millisecond
	base := server(t, cfg)

	// Offer, take the answer, and never apply it: the channel can't open.
	_, body := offer(t, newPeer(t))
	if resp := post(t, base, body); resp.StatusCode != http.StatusOK {
		t.Fatalf("first offer status %d", resp.StatusCode)
	}

	_, body = offer(t, newPeer(t))
	if resp := post(t, base, body); resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("offer over the cap: status %d, want 503", resp.StatusCode)
	}

	time.Sleep(cfg.OpenTimeout + 300*time.Millisecond)

	_, body = offer(t, newPeer(t))
	if resp := post(t, base, body); resp.StatusCode != http.StatusOK {
		t.Fatalf("offer after the abandoned peer timed out: status %d, want 200", resp.StatusCode)
	}
}
