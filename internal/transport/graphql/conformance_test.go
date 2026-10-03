package graphql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	coderws "github.com/coder/websocket"

	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/transport"
	"github.com/robbiebyrd/indri/internal/transport/transporttest"
)

// anySession resolves any token to a session whose id is the token, so the
// suite can subscribe as whichever session it names.
type anySession struct{}

func (anySession) GetByToken(token string) (*models.Session, error) {
	if token == "" {
		return nil, errors.New("no token")
	}

	return &models.Session{ID: token, Token: token}, nil
}

// subscriber is one gameUpdates subscription over graphql-transport-ws.
type subscriber struct {
	conn *coderws.Conn
}

// frame is one graphql-transport-ws message, as far as the suite reads it.
type frame struct {
	Type    string `json:"type"`
	Payload struct {
		Data struct {
			GameUpdates json.RawMessage `json:"gameUpdates"`
		} `json:"data"`
	} `json:"payload"`
}

func dialSubscription(t *testing.T, baseURL string, sessionID string) transporttest.Client {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), transporttest.Timeout)
	defer cancel()

	conn, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(baseURL, "http")+path,
		&coderws.DialOptions{Subprotocols: []string{"graphql-transport-ws"}})
	if err != nil {
		t.Fatalf("dialing %s: %v", baseURL, err)
	}

	t.Cleanup(func() { _ = conn.CloseNow() })

	s := &subscriber{conn: conn}

	s.send(t, ctx, map[string]any{"type": "connection_init", "payload": map[string]any{"Authorization": sessionID}})

	if f := s.read(t, ctx); f.Type != "connection_ack" {
		t.Fatalf("connection_init answered with %q, want connection_ack", f.Type)
	}

	s.send(t, ctx, map[string]any{
		"id":      "1",
		"type":    "subscribe",
		"payload": map[string]any{"query": `subscription { gameUpdates(gameId: "game") }`},
	})

	return s
}

func (s *subscriber) send(t *testing.T, ctx context.Context, msg map[string]any) {
	t.Helper()

	b, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("encoding %v: %v", msg, err)
	}

	if err := s.conn.Write(ctx, coderws.MessageText, b); err != nil {
		t.Fatalf("sending %s: %v", b, err)
	}
}

func (s *subscriber) read(t *testing.T, ctx context.Context) frame {
	t.Helper()

	f, err := s.next(ctx)
	if err != nil {
		t.Fatalf("reading: %v", err)
	}

	return f
}

func (s *subscriber) next(ctx context.Context) (frame, error) {
	for {
		_, b, err := s.conn.Read(ctx)
		if err != nil {
			return frame{}, err
		}

		var f frame
		if err := json.Unmarshal(b, &f); err != nil {
			return frame{}, fmt.Errorf("decoding %s: %w", b, err)
		}

		if f.Type != "ping" && f.Type != "pong" && f.Type != "ka" {
			return f, nil
		}
	}
}

// Receive returns the next gameUpdates payload. The subscription completing —
// what a kick or a closed transport does to it — ends the connection.
func (s *subscriber) Receive(timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	f, err := s.next(ctx)
	if err != nil {
		return nil, err
	}

	if f.Type != "next" {
		return nil, fmt.Errorf("the subscription ended with %q", f.Type)
	}

	return f.Payload.Data.GameUpdates, nil
}

func (s *subscriber) Close() error {
	return s.conn.Close(coderws.StatusNormalClosure, "")
}

func TestConformance(t *testing.T) {
	transporttest.Run(t, transporttest.Harness{
		New:  func(*testing.T) transport.Transport { return New(anySession{}) },
		Dial: dialSubscription,
	})
}
