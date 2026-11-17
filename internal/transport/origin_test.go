package transport

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Without an allowlist no browser origin may connect, the server's own
// included: an open default lets any page a visitor opens reach a server on
// their localhost or private network through their browser. Requests with no
// Origin (native and CLI clients) are not subject to that, and still pass.
func TestOriginPolicyAllows_WithoutAListRefusesEveryBrowserOrigin(t *testing.T) {
	policy := NewOriginPolicy("  ")

	if !policy.Allows(request("", "localhost:5004")) {
		t.Error("Allows(no origin) with no allowlist = false, want true")
	}

	for _, origin := range []string{"http://evil.example", "http://localhost:5004", "http://localhost:8081", "http://%zz"} {
		if policy.Allows(request(origin, "localhost:5004")) {
			t.Errorf("Allows(origin=%q) with no allowlist = true, want false", origin)
		}
	}
}

// "*" is the explicit opt-in to open mode: every origin may connect. It wins
// over any other entry in the same list, since a list that admits everything
// and also names some origins can only mean everything.
func TestOriginPolicyAllows_StarAcceptsEveryOrigin(t *testing.T) {
	for _, list := range []string{"*", " * ", "http://localhost:8081, *"} {
		policy := NewOriginPolicy(list)

		for _, origin := range []string{"", "http://evil.example", "http://localhost:5004", "http://%zz"} {
			if !policy.Allows(request(origin, "localhost:5004")) {
				t.Errorf("Allows(origin=%q) with list %q = false, want true", origin, list)
			}
		}
	}
}

// Boot logs the policy in force, so an operator can see from the log alone
// whether browsers are refused, listed or all admitted — and open mode, the
// one with a known risk, says so.
func TestOriginPolicyString_NamesThePolicyInForce(t *testing.T) {
	cases := map[string]struct {
		list     string
		want     string
		wantOpen bool
	}{
		"empty": {list: "", want: "closed: no browser origin is allowed; set INDRI_ALLOWED_ORIGINS to list some"},
		"list": {
			list: "http://localhost:8081, http://localhost:19006",
			want: "allowlist: http://localhost:19006, http://localhost:8081",
		},
		"star": {
			list:     "*",
			want:     "open: every browser origin is allowed (INDRI_ALLOWED_ORIGINS=\"*\")",
			wantOpen: true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			policy := NewOriginPolicy(tc.list)

			if got := policy.String(); got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}

			if got := policy.Open(); got != tc.wantOpen {
				t.Errorf("Open() = %v, want %v", got, tc.wantOpen)
			}
		})
	}
}

// With an allowlist, the list is the whole policy: its origins, and requests
// with no Origin (native and CLI clients).
func TestOriginPolicyAllows_WithAListAcceptsOnlyItAndNativeClients(t *testing.T) {
	policy := NewOriginPolicy(" http://localhost:8081 ,, http://localhost:19006 ")

	cases := map[string]struct {
		origin string
		host   string
		want   bool
	}{
		"no origin (native client)": {origin: "", want: true},
		"listed origin":             {origin: "http://localhost:8081", want: true},
		"second listed origin":      {origin: "http://localhost:19006", want: true},
		"unlisted origin":           {origin: "http://evil.example", want: false},
		"malformed origin":          {origin: "http://%zz", want: false},

		// The server's own origin gets no pass: a DNS-rebinding page presents
		// exactly that. A React Native iOS client, which sends it, must be
		// listed.
		"the server's own origin, unlisted": {origin: "http://localhost:5004", host: "localhost:5004", want: false},
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

// A nil policy is a transport built without one — a wiring bug, not a choice
// of configuration — so it must not mean "open to everyone": it still rejects
// cross-origin.
func TestOriginPolicyAllows_NilPolicyRejectsCrossOrigin(t *testing.T) {
	var policy *OriginPolicy

	if !policy.Allows(request("", "")) {
		t.Error("Allows(no origin) = false, want true")
	}

	if policy.Allows(request("http://evil.example", "")) {
		t.Error("Allows(cross origin) = true, want false")
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
