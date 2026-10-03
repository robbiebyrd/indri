package ws

import (
	"context"
	"strings"
	"testing"
	"time"

	coderws "github.com/coder/websocket"

	"github.com/robbiebyrd/indri/internal/transport"
	"github.com/robbiebyrd/indri/internal/transport/transporttest"
)

// wsClient is a WebSocket client connection. The WebSocket transport learns who
// a client is when it logs in, not when it connects, so Dial ignores the
// session; the suite tags the server-side conn itself.
type wsClient struct {
	conn *coderws.Conn
}

func dialWS(t *testing.T, baseURL string, _ string) transporttest.Client {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), transporttest.Timeout)
	defer cancel()

	conn, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(baseURL, "http")+path, nil)
	if err != nil {
		t.Fatalf("dialing %s: %v", baseURL, err)
	}

	t.Cleanup(func() { _ = conn.CloseNow() })

	return &wsClient{conn: conn}
}

func (c *wsClient) Receive(timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	_, msg, err := c.conn.Read(ctx)

	return msg, err
}

func (c *wsClient) Send(msg []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), transporttest.Timeout)
	defer cancel()

	return c.conn.Write(ctx, coderws.MessageText, msg)
}

func (c *wsClient) Close() error {
	return c.conn.Close(coderws.StatusNormalClosure, "")
}

func TestConformance(t *testing.T) {
	transporttest.Run(t, transporttest.Harness{
		New:    func(*testing.T) transport.Transport { return New() },
		Dial:   dialWS,
		TwoWay: true,
	})
}
