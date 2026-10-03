package webrtc

import (
	"errors"
	"testing"
	"time"

	pion "github.com/pion/webrtc/v4"

	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/transport"
	"github.com/robbiebyrd/indri/internal/transport/transporttest"
)

// anySession resolves any token to a session whose id is the token, so the
// suite can connect as whichever session it names.
type anySession struct{}

func (anySession) GetByToken(token string) (*models.Session, error) {
	if token == "" {
		return nil, errors.New("no token")
	}

	return &models.Session{ID: token, Token: token}, nil
}

// rtcClient is a pion client talking to the transport over its game channel.
type rtcClient struct {
	pc       *pion.PeerConnection
	game     *pion.DataChannel
	messages chan []byte
	ended    chan struct{}
}

func dialPeer(t *testing.T, baseURL string, sessionID string) transporttest.Client {
	t.Helper()

	pc, channels := clientHandshake(t, baseURL+signalPath, sessionID, "game")

	p := &rtcClient{pc: pc, game: channels["game"], messages: make(chan []byte, 64), ended: make(chan struct{})}

	p.game.OnMessage(func(msg pion.DataChannelMessage) { p.messages <- msg.Data })
	p.game.OnClose(func() { close(p.ended) })

	return p
}

func (p *rtcClient) Receive(timeout time.Duration) ([]byte, error) {
	select {
	case msg := <-p.messages:
		return msg, nil
	default:
	}

	select {
	case msg := <-p.messages:
		return msg, nil
	case <-p.ended:
		return nil, errors.New("the data channel closed")
	case <-time.After(timeout):
		return nil, errors.New("timed out")
	}
}

func (p *rtcClient) Send(msg []byte) error {
	return p.game.Send(msg)
}

func (p *rtcClient) Close() error {
	return p.pc.Close()
}

func TestConformance(t *testing.T) {
	transporttest.Run(t, transporttest.Harness{
		New: func(t *testing.T) transport.Transport {
			tr, err := New(anySession{}, Config{UDPPort: 0})
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			return tr
		},
		Dial:   dialPeer,
		TwoWay: true,
	})
}
