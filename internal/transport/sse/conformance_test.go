package sse

import (
	"bufio"
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/transport"
	"github.com/robbiebyrd/indri/internal/transport/transporttest"
)

// everySession resolves any token to a session whose id is the token, so the
// suite can authenticate a stream as whichever session it names.
type everySession struct{}

func (everySession) GetByToken(token string) (*models.Session, error) {
	if token == "" {
		return nil, errors.New("no token")
	}

	return &models.Session{ID: token, Token: token}, nil
}

// sseClient reads one event stream, handing each data frame to Receive.
type sseClient struct {
	cancel context.CancelFunc
	frames chan []byte
	ended  chan struct{}
}

func dialSSE(t *testing.T, baseURL string, sessionID string) transporttest.Client {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path, nil)
	if err != nil {
		cancel()
		t.Fatalf("building request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+sessionID)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("opening stream: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		cancel()
		t.Fatalf("stream status = %d, want 200", resp.StatusCode)
	}

	c := &sseClient{cancel: cancel, frames: make(chan []byte, 64), ended: make(chan struct{})}

	go func() {
		defer close(c.ended)
		defer resp.Body.Close()

		r := bufio.NewReader(resp.Body)

		var data []string

		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}

			line = strings.TrimRight(line, "\n")

			switch {
			case strings.HasPrefix(line, "data: "):
				data = append(data, strings.TrimPrefix(line, "data: "))
			case line == "" && data != nil:
				c.frames <- []byte(strings.Join(data, "\n"))
				data = nil
			}
		}
	}()

	t.Cleanup(cancel)

	return c
}

func (c *sseClient) Receive(timeout time.Duration) ([]byte, error) {
	select {
	case frame := <-c.frames:
		return frame, nil
	default:
	}

	select {
	case frame := <-c.frames:
		return frame, nil
	case <-c.ended:
		return nil, errors.New("the stream ended")
	case <-time.After(timeout):
		return nil, errors.New("timed out")
	}
}

func (c *sseClient) Close() error {
	c.cancel()

	return nil
}

func TestConformance(t *testing.T) {
	transporttest.Run(t, transporttest.Harness{
		New:  func(*testing.T) transport.Transport { return New(everySession{}) },
		Dial: dialSSE,
	})
}
