package graphql

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	coderws "github.com/coder/websocket"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
	"github.com/robbiebyrd/indri/internal/handlers/router"
	"github.com/robbiebyrd/indri/internal/transport"
)

// The gameUpdates subscription is a WebSocket upgrade, and gqlgen's upgrader
// defaults to same-origin only. Without the shared policy an allowlisted
// browser clears the CORS middleware, sends its mutations happily, and then
// silently never receives a delta.
func TestRegister_SubscriptionUpgradeUsesTheOriginPolicy(t *testing.T) {
	cases := map[string]struct {
		origin     string
		sameOrigin bool
		allowed    bool
	}{
		"allowlisted cross origin": {origin: "http://localhost:8081", allowed: true},
		"same origin":              {sameOrigin: true, allowed: true},
		"no origin (native)":       {allowed: true},
		"disallowed origin":        {origin: "http://evil.example"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			tr := New(nil)
			tr.origins = transport.NewOriginPolicy("http://localhost:8081")

			mux := http.NewServeMux()
			tr.Register(mux)

			server := httptest.NewServer(mux)
			defer server.Close()

			origin := tc.origin
			if tc.sameOrigin {
				origin = server.URL
			}

			header := http.Header{}
			if origin != "" {
				header.Set("Origin", origin)
			}

			conn, resp, err := coderws.Dial(
				context.Background(),
				"ws"+strings.TrimPrefix(server.URL, "http")+path,
				&coderws.DialOptions{HTTPHeader: header},
			)
			if conn != nil {
				defer conn.CloseNow()
			}

			// On a successful handshake the dialler returns the 101 response
			// with no body to drain.
			if resp != nil && resp.Body != nil {
				defer resp.Body.Close()
			}

			if tc.allowed {
				if err != nil {
					t.Fatalf("Dial(origin=%q) = %v, want the upgrade to succeed", origin, err)
				}

				return
			}

			if err == nil {
				t.Fatalf("Dial(origin=%q) succeeded, want it refused", origin)
			}

			// gqlgen answers a refused upgrade itself, with 400. Served through
			// internal/entrypoints/http the CORS middleware refuses this origin
			// with 403 before the request ever reaches the transport; this is
			// the second line of defence, so what matters is that no connection
			// is established.
			if resp == nil || resp.StatusCode != http.StatusBadRequest {
				t.Errorf("Dial(origin=%q) status = %v, want %d", origin, resp, http.StatusBadRequest)
			}
		})
	}
}

// A GraphQL operation is an HTTP body like any other, and until it was capped
// this was the one inbound surface with no limit: REST wraps its bodies in a
// MaxBytesReader and the WebSocket transport sets MaxMessageSize, so an
// oversized layout op — a script source, an opaque config — had exactly one way
// in. The cap has to refuse it before the executor parses or resolves anything,
// and has to leave an ordinary operation untouched.
func TestRegister_BodyLimit(t *testing.T) {
	// pad makes a valid { ping } operation of roughly the requested size by
	// padding an unused variable. It stays valid GraphQL, so "pong" in the
	// response means the resolver ran and the cap failed to stop it.
	pad := func(size int) string {
		body, err := json.Marshal(map[string]interface{}{
			"query":     "{ ping }",
			"variables": map[string]string{"pad": strings.Repeat("a", size)},
		})
		if err != nil {
			t.Fatalf("Marshal() = %v", err)
		}

		return string(body)
	}

	cases := map[string]struct {
		body string
		// chunked sends the body without a Content-Length, so the limit can
		// only be enforced as the handler reads.
		chunked bool
		// wantStatus of 0 is not asserted: gqlgen answers a body it could not
		// read with its own error document, and the status it picks for that is
		// its business. What matters there is that no resolver ran.
		wantStatus int
		wantPong   bool
	}{
		"ordinary operation": {
			body:       pad(64),
			wantStatus: http.StatusOK,
			wantPong:   true,
		},
		"just inside the limit": {
			body:       pad(transport.MaxBodyBytes - 512),
			wantStatus: http.StatusOK,
			wantPong:   true,
		},
		"over the limit": {
			body:       pad(transport.MaxBodyBytes),
			wantStatus: http.StatusRequestEntityTooLarge,
		},
		"over the limit without a declared length": {
			body:    pad(transport.MaxBodyBytes),
			chunked: true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			tr := New(nil)

			mux := http.NewServeMux()
			tr.Register(mux)

			server := httptest.NewServer(mux)
			defer server.Close()

			req, err := http.NewRequest(http.MethodPost, server.URL+path, strings.NewReader(tc.body))
			if err != nil {
				t.Fatalf("NewRequest() = %v", err)
			}

			req.Header.Set("Content-Type", "application/json")

			if tc.chunked {
				// A negative length is what net/http reads as "unknown", which
				// makes it send the body chunked.
				req.ContentLength = -1
			}

			resp, err := server.Client().Do(req)
			if err != nil {
				t.Fatalf("Do() = %v", err)
			}
			defer resp.Body.Close()

			document, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("ReadAll() = %v", err)
			}

			if tc.wantStatus != 0 && resp.StatusCode != tc.wantStatus {
				t.Errorf("status = %d, want %d (body %q)", resp.StatusCode, tc.wantStatus, document)
			}

			// The resolver returns "pong" and nothing else in this schema does,
			// so its absence is proof the operation never executed.
			if gotPong := strings.Contains(string(document), "pong"); gotPong != tc.wantPong {
				t.Errorf("response %q contains pong = %v, want %v", document, gotPong, tc.wantPong)
			}
		})
	}
}

// The subscription shares the endpoint with the mutations, so the body cap sits
// in front of its upgrade too. An upgrade carries no body, but it does need the
// ResponseWriter to stay hijackable — wrapping it would break every push client
// while leaving the mutations looking healthy.
func TestRegister_BodyLimitLeavesTheSubscriptionUpgradeAlone(t *testing.T) {
	tr := New(nil)
	tr.origins = transport.NewOriginPolicy("")

	mux := http.NewServeMux()
	tr.Register(mux)

	server := httptest.NewServer(mux)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, resp, err := coderws.Dial(
		ctx,
		"ws"+strings.TrimPrefix(server.URL, "http")+path,
		&coderws.DialOptions{Subprotocols: []string{"graphql-transport-ws"}},
	)
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}

	if err != nil {
		t.Fatalf("Dial() = %v, want the upgrade to succeed", err)
	}

	defer conn.CloseNow()

	// The handshake alone only proves the hijack worked. Completing
	// connection_init proves the socket carries traffic in both directions.
	if err := conn.Write(ctx, coderws.MessageText, []byte(`{"type":"connection_init"}`)); err != nil {
		t.Fatalf("Write(connection_init) = %v", err)
	}

	ack, err := readMessageType(ctx, conn)
	if err != nil {
		t.Fatalf("reading the reply to connection_init: %v", err)
	}

	if ack != "connection_ack" {
		t.Errorf("reply to connection_init = %q, want %q", ack, "connection_ack")
	}
}

// readMessageType reads one graphql-transport-ws frame and returns its type.
func readMessageType(ctx context.Context, conn *coderws.Conn) (string, error) {
	_, data, err := conn.Read(ctx)
	if err != nil {
		return "", fmt.Errorf("reading a frame: %w", err)
	}

	var message struct {
		Type string `json:"type"`
	}

	if err := json.Unmarshal(data, &message); err != nil {
		return "", fmt.Errorf("decoding %q: %w", data, err)
	}

	return message.Type, nil
}

// staticHandler returns one response document, the way every built-in action
// does. Two of them registered under the same action is the multi-handler
// dispatch router.Dispatch merges — a game handler alongside a built-in, a Lua
// hook, or a received/processed pre/post hook.
type staticHandler struct{ document string }

func (h staticHandler) Handle(actions.Request) (actions.Result, error) {
	return actions.Result{Responses: [][]byte{[]byte(h.document)}}, nil
}

// syncBuffer is a log sink safe to read from the test goroutine while the
// server goroutine writes to it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.String()
}

func captureLog(t *testing.T) *syncBuffer {
	t.Helper()

	buf := &syncBuffer{}
	out := log.Writer()

	log.SetOutput(buf)
	t.Cleanup(func() { log.SetOutput(out) })

	return buf
}

// mutate runs one operation against a server carrying only the registry the
// test populated, and returns the decoded response.
func mutate(t *testing.T, query string) map[string]interface{} {
	t.Helper()

	tr := New(nil)

	mux := http.NewServeMux()
	tr.Register(mux)

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	body, err := json.Marshal(map[string]interface{}{"query": query})
	if err != nil {
		t.Fatalf("Marshal() = %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, server.URL+path, strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("NewRequest() = %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("Do() = %v", err)
	}
	defer resp.Body.Close()

	document, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll() = %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(document, &decoded); err != nil {
		t.Fatalf("Unmarshal(%q) = %v", document, err)
	}

	if errs, ok := decoded["errors"]; ok {
		t.Fatalf("the operation returned errors: %v", errs)
	}

	return decoded
}

// A mutation resolves to one value, so a dispatch that produced two responses
// can deliver only the first. That is the contract (docs/PROTOCOL.md), and the
// WebSocket transport deliberately differs — see
// TestHandleClientMessage_WritesEveryResponseOfAMultiHandlerDispatch in
// internal/services/boot. What must never happen is the drop going unreported.
func TestMutation_MultiHandlerActionResolvesToTheFirstResponseAndReportsTheRest(t *testing.T) {
	router.Reset()
	t.Cleanup(router.Reset)

	router.RegisterHandler("test_logout_first", "logout", staticHandler{`{"first":true}`})
	router.RegisterHandler("test_logout_second", "logout", staticHandler{`{"second":true}`})

	logged := captureLog(t)

	decoded := mutate(t, "mutation { logout }")

	data, _ := decoded["data"].(map[string]interface{})

	want := map[string]interface{}{"first": true}
	if got := data["logout"]; !reflect.DeepEqual(got, want) {
		t.Errorf("logout = %v, want %v (the first response, not the merged set)", got, want)
	}

	report := logged.String()

	if !strings.Contains(report, `"logout"`) {
		t.Errorf("the dropped response was not reported (log was %q); a silent drop is "+
			"indistinguishable from a handler that never ran", report)
	}
}

// The single-response case is every action today, and it must stay exactly as
// it was: one document, and nothing logged.
func TestMutation_SingleResponseActionIsUnchangedAndSilent(t *testing.T) {
	router.Reset()
	t.Cleanup(router.Reset)

	router.RegisterHandler("test_logout_only", "logout", staticHandler{`{"only":true}`})

	logged := captureLog(t)

	decoded := mutate(t, "mutation { logout }")

	data, _ := decoded["data"].(map[string]interface{})

	want := map[string]interface{}{"only": true}
	if got := data["logout"]; !reflect.DeepEqual(got, want) {
		t.Errorf("logout = %v, want %v", got, want)
	}

	if report := logged.String(); report != "" {
		t.Errorf("a single-response action logged %q, want nothing", report)
	}
}
