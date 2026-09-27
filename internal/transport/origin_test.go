package transport_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/robbiebyrd/indri/internal/transport"
)

func TestOriginChecker(t *testing.T) {
	check := transport.OriginChecker(" https://a.example , https://b.example,")

	cases := []struct {
		name   string
		origin string
		want   bool
	}{
		{"no origin is a native client", "", true},
		{"allowlisted origin", "https://a.example", true},
		{"second allowlisted origin, whitespace trimmed", "https://b.example", true},
		{"unlisted origin", "https://evil.example", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}

			if got := check(r); got != tc.want {
				t.Errorf("OriginChecker(%q) = %v, want %v", tc.origin, got, tc.want)
			}
		})
	}
}

func TestRoute_CORS(t *testing.T) {
	mux := http.NewServeMux()

	called := 0
	transport.Route(mux, "https://app.example", http.MethodPost, "/thing",
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called++
			w.WriteHeader(http.StatusAccepted)
		}))

	t.Run("preflight from allowed origin", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodOptions, "/thing", nil)
		r.Header.Set("Origin", "https://app.example")
		r.Header.Set("Access-Control-Request-Method", http.MethodPost)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)

		if w.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusNoContent)
		}
		if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example" {
			t.Errorf("Allow-Origin = %q", got)
		}
		if got := w.Header().Get("Access-Control-Allow-Methods"); got != http.MethodPost {
			t.Errorf("Allow-Methods = %q, want %q", got, http.MethodPost)
		}
		if got := w.Header().Get("Access-Control-Allow-Headers"); got == "" {
			t.Error("Allow-Headers missing; the connection-id header would be refused by the browser")
		}
	})

	t.Run("request from allowed origin reaches handler with CORS headers", func(t *testing.T) {
		called = 0
		r := httptest.NewRequest(http.MethodPost, "/thing", nil)
		r.Header.Set("Origin", "https://app.example")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)

		if called != 1 || w.Code != http.StatusAccepted {
			t.Fatalf("called=%d status=%d", called, w.Code)
		}
		if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example" {
			t.Errorf("Allow-Origin = %q", got)
		}
		if got := w.Header().Get("Vary"); got != "Origin" {
			t.Errorf("Vary = %q, want Origin", got)
		}
	})

	t.Run("request from unlisted origin is refused before the handler", func(t *testing.T) {
		called = 0
		r := httptest.NewRequest(http.MethodPost, "/thing", nil)
		r.Header.Set("Origin", "https://evil.example")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)

		if called != 0 {
			t.Error("handler ran for a disallowed origin")
		}
		if w.Code != http.StatusForbidden {
			t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
		}
	})

	t.Run("request without origin (native client) passes, no CORS headers", func(t *testing.T) {
		called = 0
		r := httptest.NewRequest(http.MethodPost, "/thing", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)

		if called != 1 {
			t.Error("native request did not reach handler")
		}
		if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("Allow-Origin = %q, want none", got)
		}
	})

	t.Run("wrong method is rejected by the mux", func(t *testing.T) {
		called = 0
		r := httptest.NewRequest(http.MethodGet, "/thing", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)

		if called != 0 || w.Code != http.StatusMethodNotAllowed {
			t.Errorf("called=%d status=%d, want 0/%d", called, w.Code, http.StatusMethodNotAllowed)
		}
	})
}
