package graphql

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	coderws "github.com/coder/websocket"

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
