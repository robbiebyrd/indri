package ws

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	coderws "github.com/coder/websocket"

	"github.com/robbiebyrd/indri/internal/services/events"
	"github.com/robbiebyrd/indri/internal/transport"
)

// gameDeltas is a whole tic-tac-toe game as the server broadcasts it: nine
// moves, each a board cell and the two turn flags. It is the session story 083
// measured, so the figures here can be set beside that one.
func gameDeltas(t *testing.T) [][]byte {
	t.Helper()

	cells := [][3]any{
		{0, 0, "X"}, {1, 1, "O"}, {0, 1, "X"}, {2, 2, "O"}, {0, 2, "X"},
		{1, 0, "O"}, {2, 0, "X"}, {1, 2, "O"}, {2, 1, "X"},
	}

	ts := time.Date(2026, 10, 1, 14, 3, 12, 482_000_000, time.UTC)
	turn := true

	msgs := make([][]byte, 0, len(cells))

	for _, cell := range cells {
		msg, err := json.Marshal(events.ChangeEvent{
			ID:            "66fb9a1c2e4d5f6a7b8c9d0e",
			OperationType: "update",
			Timestamp:     ts,
			Collection:    "game",
			UpdatedFields: map[string]interface{}{
				fmt.Sprintf("stage.scenes.board.data.board.%d.%d", cell[0], cell[1]): cell[2],
				"teams.Player 1.data.turn": !turn,
				"teams.Player 2.data.turn": turn,
			},
		})
		if err != nil {
			t.Fatalf("encoding a delta: %v", err)
		}

		msgs = append(msgs, msg)
		turn = !turn
	}

	return msgs
}

// frame is one server-to-client WebSocket frame as it crossed the wire.
type frame struct {
	wireBytes  int
	compressed bool // RSV1: the frame's payload is deflated
}

// handshake opens a raw TCP connection to the server's /ws route, asks for
// the extensions given (none when empty), and returns the reader positioned at
// the first frame and the server's answer to the extension offer.
func handshake(t *testing.T, server *httptest.Server, extensions string) (*bufio.Reader, string) {
	t.Helper()

	c, err := net.Dial("tcp", strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatalf("dialing %s: %v", server.URL, err)
	}

	t.Cleanup(func() { _ = c.Close() })

	_ = c.SetDeadline(time.Now().Add(5 * time.Second))

	request := "GET " + path + " HTTP/1.1\r\n" +
		"Host: " + c.RemoteAddr().String() + "\r\n" +
		"Upgrade: websocket\r\nConnection: Upgrade\r\n" +
		"Sec-WebSocket-Version: 13\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n"
	if extensions != "" {
		request += "Sec-WebSocket-Extensions: " + extensions + "\r\n"
	}

	if _, err := io.WriteString(c, request+"\r\n"); err != nil {
		t.Fatalf("sending the upgrade: %v", err)
	}

	r := bufio.NewReader(c)

	resp, err := http.ReadResponse(r, nil)
	if err != nil {
		t.Fatalf("reading the upgrade response: %v", err)
	}

	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("upgrade answered %s, want 101", resp.Status)
	}

	return r, resp.Header.Get("Sec-WebSocket-Extensions")
}

// readFrames reads n server frames, counting every byte each took on the
// wire: header, extended length and payload. Server frames are never masked.
func readFrames(t *testing.T, r *bufio.Reader, n int) []frame {
	t.Helper()

	frames := make([]frame, 0, n)

	for range n {
		var head [2]byte
		if _, err := io.ReadFull(r, head[:]); err != nil {
			t.Fatalf("reading frame %d: %v", len(frames)+1, err)
		}

		wire := 2
		size := int(head[1] & 0x7f)

		switch size {
		case 126:
			var ext [2]byte
			if _, err := io.ReadFull(r, ext[:]); err != nil {
				t.Fatalf("reading frame %d length: %v", len(frames)+1, err)
			}

			size, wire = int(binary.BigEndian.Uint16(ext[:])), wire+2
		case 127:
			var ext [8]byte
			if _, err := io.ReadFull(r, ext[:]); err != nil {
				t.Fatalf("reading frame %d length: %v", len(frames)+1, err)
			}

			size, wire = int(binary.BigEndian.Uint64(ext[:])), wire+8
		}

		if _, err := io.CopyN(io.Discard, r, int64(size)); err != nil {
			t.Fatalf("reading frame %d payload: %v", len(frames)+1, err)
		}

		frames = append(frames, frame{wireBytes: wire + size, compressed: head[0]&0x40 != 0})
	}

	return frames
}

// serveDeltas runs a WebSocket transport that sends every connection msgs.
func serveDeltas(t *testing.T, msgs [][]byte) *httptest.Server {
	t.Helper()

	tr := New()
	tr.Handle(transport.Handlers{Connect: func(c transport.Conn) {
		for _, msg := range msgs {
			_ = c.Write(msg)
		}
	}})

	mux := http.NewServeMux()
	tr.Register(mux)

	server := httptest.NewServer(mux)
	t.Cleanup(func() {
		server.Close()
		_ = tr.Close()
	})

	return server
}

// A client that offers permessage-deflate gets it, and what it is then sent is
// actually compressed — the extension answered in the handshake is worth
// nothing if every frame still goes out raw.
func TestCompression_NegotiatedWhenTheClientOffersIt(t *testing.T) {
	msgs := gameDeltas(t)
	server := serveDeltas(t, msgs)

	r, accepted := handshake(t, server, "permessage-deflate; client_max_window_bits")
	if !strings.Contains(accepted, "permessage-deflate") {
		t.Fatalf("Sec-WebSocket-Extensions answered %q, want permessage-deflate", accepted)
	}

	// Without context takeover each delta is deflated alone, and a game's
	// deltas shrink by about a quarter instead of most of the way: the
	// repetition deflate removes is between messages, not inside one.
	if strings.Contains(accepted, "server_no_context_takeover") {
		t.Fatalf("Sec-WebSocket-Extensions answered %q: the server refuses context takeover", accepted)
	}

	raw, wire := 0, 0

	for i, f := range readFrames(t, r, len(msgs)) {
		if !f.compressed {
			t.Errorf("frame %d went out uncompressed on a connection that negotiated deflate", i+1)
		}

		raw += len(msgs[i])
		wire += f.wireBytes
	}

	if wire >= raw {
		t.Errorf("a game's deltas took %d bytes on the wire compressed, no fewer than %d raw", wire, raw)
	}

	t.Logf("a nine-move game: %d bytes of delta JSON, %d bytes on the wire with permessage-deflate", raw, wire)
}

// Compression is offered, never required: a client that does not ask still
// connects, and is sent plain frames it can read.
func TestCompression_AClientThatDoesNotAskConnectsUncompressed(t *testing.T) {
	msgs := gameDeltas(t)
	server := serveDeltas(t, msgs)

	r, accepted := handshake(t, server, "")
	if accepted != "" {
		t.Fatalf("Sec-WebSocket-Extensions answered %q to a client that offered none", accepted)
	}

	raw, wire := 0, 0

	for i, f := range readFrames(t, r, len(msgs)) {
		if f.compressed {
			t.Errorf("frame %d is compressed on a connection that never negotiated it", i+1)
		}

		raw += len(msgs[i])
		wire += f.wireBytes
	}

	t.Logf("a nine-move game: %d bytes of delta JSON, %d bytes on the wire uncompressed", raw, wire)
}

// The read limit is on what a message inflates to, not on the bytes it took on
// the wire. Zeros deflate to almost nothing, so a limit counted on the wire
// would let a client hand the server an allocation of any size it liked in one
// small frame.
func TestCompression_ReadLimitBoundsTheInflatedMessage(t *testing.T) {
	var delivered atomic.Int32

	// A limit below the library's own default, so the test fails if the
	// configured limit is never applied.
	tr := New()
	tr.maxMessageSize = 1024
	tr.Handle(transport.Handlers{Message: func(transport.Conn, []byte) { delivered.Add(1) }})

	mux := http.NewServeMux()
	tr.Register(mux)

	server := httptest.NewServer(mux)
	t.Cleanup(func() {
		server.Close()
		_ = tr.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+path,
		&coderws.DialOptions{CompressionMode: coderws.CompressionContextTakeover})
	if err != nil {
		t.Fatalf("dialing: %v", err)
	}

	defer client.CloseNow()

	client.SetReadLimit(-1)

	bomb := make([]byte, 16<<10)
	if err := client.Write(ctx, coderws.MessageText, bomb); err != nil {
		t.Fatalf("sending the message: %v", err)
	}

	_, _, err = client.Read(ctx)
	if status := coderws.CloseStatus(err); status != coderws.StatusMessageTooBig {
		t.Errorf("the server answered a %d-byte inflated message with %v (close status %v), want %v",
			len(bomb), err, status, coderws.StatusMessageTooBig)
	}

	if errors.Is(err, context.DeadlineExceeded) || delivered.Load() != 0 {
		t.Errorf("an over-limit message was delivered %d times", delivered.Load())
	}
}
