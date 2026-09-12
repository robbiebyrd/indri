package transport

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOriginPolicyAllows(t *testing.T) {
	policy := NewOriginPolicy(" http://localhost:8081 ,, http://localhost:19006 ")

	cases := map[string]struct {
		origin string
		host   string
		want   bool
	}{
		"no origin (native client)":  {origin: "", want: true},
		"allowed origin":             {origin: "http://localhost:8081", want: true},
		"second allowed origin":      {origin: "http://localhost:19006", want: true},
		"disallowed origin":          {origin: "http://evil.example", want: false},
		"empty entry is not a match": {origin: "", want: true},

		// React Native's WebSocket derives an Origin from the target URL, so a
		// native client dialling the server sends the server's own origin.
		// Same-origin is not cross-site, so no allowlist entry is needed.
		"same origin as the server":       {origin: "http://localhost:5004", host: "localhost:5004", want: true},
		"same host over https":            {origin: "https://indri.example", host: "indri.example", want: true},
		"same host, different port":       {origin: "http://localhost:5005", host: "localhost:5004", want: false},
		"origin host is a prefix of host": {origin: "http://localhost", host: "localhost:5004", want: false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := request(tc.origin, tc.host)

			if got := policy.Allows(r); got != tc.want {
				t.Errorf("Allows(origin=%q, host=%q) = %v, want %v", tc.origin, r.Host, got, tc.want)
			}
		})
	}
}

// The policy is the same for every transport, so a nil one must not mean "open
// to everyone": a transport built without one still rejects cross-origin.
func TestOriginPolicyAllows_NilPolicyRejectsCrossOrigin(t *testing.T) {
	var policy *OriginPolicy

	if !policy.Allows(request("", "")) {
		t.Error("Allows(no origin) = false, want true")
	}

	if policy.Allows(request("http://evil.example", "")) {
		t.Error("Allows(cross origin) = true, want false")
	}

	if !policy.Allows(request("http://localhost:5004", "localhost:5004")) {
		t.Error("Allows(same origin) = false, want true")
	}
}

func TestOriginPolicyMiddleware(t *testing.T) {
	policy := NewOriginPolicy("http://localhost:8081")

	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})

	cases := map[string]struct {
		method    string
		origin    string
		preflight bool
		status    int
		allowOrig string
		reached   bool
	}{
		"allowed cross origin is echoed back": {
			method:    http.MethodPost,
			origin:    "http://localhost:8081",
			status:    http.StatusTeapot,
			allowOrig: "http://localhost:8081",
			reached:   true,
		},
		"disallowed origin never reaches the handler": {
			method: http.MethodPost,
			origin: "http://evil.example",
			status: http.StatusForbidden,
		},
		"no origin passes through without CORS headers": {
			method:  http.MethodPost,
			status:  http.StatusTeapot,
			reached: true,
		},
		"preflight is answered by the middleware": {
			method:    http.MethodOptions,
			origin:    "http://localhost:8081",
			preflight: true,
			status:    http.StatusNoContent,
			allowOrig: "http://localhost:8081",
		},
		"preflight from a disallowed origin is refused": {
			method:    http.MethodOptions,
			origin:    "http://evil.example",
			preflight: true,
			status:    http.StatusForbidden,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, "/graphql", nil)
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}

			if tc.preflight {
				r.Header.Set("Access-Control-Request-Method", http.MethodPost)
			}

			w := httptest.NewRecorder()
			policy.Middleware(next).ServeHTTP(w, r)

			if w.Code != tc.status {
				t.Errorf("status = %d, want %d", w.Code, tc.status)
			}

			if got := w.Header().Get("Access-Control-Allow-Origin"); got != tc.allowOrig {
				t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, tc.allowOrig)
			}

			// Without Vary, a shared cache can serve one origin's CORS headers
			// to a browser on a different origin.
			if got := w.Header().Get("Vary"); got != "Origin" {
				t.Errorf("Vary = %q, want %q", got, "Origin")
			}

			if reached := w.Code == http.StatusTeapot; reached != tc.reached {
				t.Errorf("handler reached = %v, want %v", reached, tc.reached)
			}
		})
	}
}

// A preflight has to advertise what the real request is allowed to send, or the
// browser blocks it even though the origin is allowed.
func TestOriginPolicyMiddleware_PreflightAdvertisesMethodsAndHeaders(t *testing.T) {
	r := httptest.NewRequest(http.MethodOptions, "/graphql", nil)
	r.Header.Set("Origin", "http://localhost:8081")
	r.Header.Set("Access-Control-Request-Method", http.MethodPost)

	w := httptest.NewRecorder()
	NewOriginPolicy("http://localhost:8081").
		Middleware(http.NotFoundHandler()).
		ServeHTTP(w, r)

	for header, want := range map[string]string{
		"Access-Control-Allow-Methods": "GET, POST, OPTIONS",
		"Access-Control-Allow-Headers": "Authorization, Content-Type",
	} {
		if got := w.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
}

func request(origin, host string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if origin != "" {
		r.Header.Set("Origin", origin)
	}

	if host != "" {
		r.Host = host
	}

	return r
}
