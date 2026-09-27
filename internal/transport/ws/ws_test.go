package ws_test

import (
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/robbiebyrd/indri/internal/transport"
	"github.com/robbiebyrd/indri/internal/transport/transporttest"
	"github.com/robbiebyrd/indri/internal/transport/ws"
)

type client struct {
	c *websocket.Conn
}

func (c *client) Send(msg []byte) error {
	return c.c.WriteMessage(websocket.TextMessage, msg)
}

func (c *client) Receive(timeout time.Duration) (transporttest.Frame, error) {
	_ = c.c.SetReadDeadline(time.Now().Add(timeout))

	kind, data, err := c.c.ReadMessage()
	if err != nil {
		return transporttest.Frame{}, err
	}

	return transporttest.Frame{Data: data, Binary: kind == websocket.BinaryMessage}, nil
}

func (c *client) Close() error { return c.c.Close() }

func TestConformance(t *testing.T) {
	transporttest.Run(t, transporttest.Harness{
		New: func(*testing.T) transport.Transport { return ws.New() },
		Dial: func(t *testing.T, baseURL string) transporttest.Client {
			url := "ws" + strings.TrimPrefix(baseURL, "http") + "/ws"

			c, _, err := websocket.DefaultDialer.Dial(url, nil)
			if err != nil {
				t.Fatalf("dial: %v", err)
			}

			return &client{c}
		},
	})
}
