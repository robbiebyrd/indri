package ws

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/robbiebyrd/indri/internal/transport"
)

// serve runs one request through the transport's registered /ws route.
func serve(t *testing.T, tr *Transport, r *http.Request) *httptest.ResponseRecorder {
	t.Helper()

	mux := http.NewServeMux()
	tr.Register(mux)

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	return w
}

// A failed upgrade is answered once, by the upgrader. Answering it a second
// time is what produces "superfluous response.WriteHeader call" at runtime,
// and it corrupts the body the client reads.
func TestRegister_FailedUpgradeIsNotAnsweredTwice(t *testing.T) {
	cases := map[string]struct {
		handshake bool
		origin    string
		status    int
	}{
		"not a websocket handshake": {
			status: http.StatusBadRequest,
		},
		"origin not allowed": {
			handshake: true,
			origin:    "http://evil.example",
			status:    http.StatusForbidden,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			tr := New()
			tr.m.Upgrader.CheckOrigin = transport.NewOriginPolicy("http://localhost:8081").Allows

			r := httptest.NewRequest(http.MethodGet, path, nil)
			if tc.handshake {
				r.Header.Set("Connection", "Upgrade")
				r.Header.Set("Upgrade", "websocket")
				r.Header.Set("Sec-WebSocket-Version", "13")
				r.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
			}

			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}

			w := serve(t, tr, r)

			// The upgrader answers with the bare status text; anything appended
			// to it is a second, superfluous write from the route.
			want := http.StatusText(tc.status) + "\n"

			if w.Code != tc.status {
				t.Errorf("status = %d, want %d", w.Code, tc.status)
			}

			if got := w.Body.String(); got != want {
				t.Errorf("body = %q, want %q", got, want)
			}
		})
	}
}

// A closed transport rejects before the upgrader writes anything, so the
// route owns the response.
func TestRegister_ClosedTransportRespondsUnavailable(t *testing.T) {
	tr := New()
	if err := tr.Close(); err != nil {
		t.Fatalf("Close() = %v", err)
	}

	w := serve(t, tr, httptest.NewRequest(http.MethodGet, path, nil))

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}

	if got := w.Body.String(); !strings.Contains(got, "closed") {
		t.Errorf("body = %q, want it to mention the transport is closed", got)
	}
}
