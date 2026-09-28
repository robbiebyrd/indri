package transport

import (
	"net/http"
	"net/url"
	"strings"
)

// ConnectionIDHeader correlates an HTTP request with an already-open
// connection on transports whose two directions travel separately (SSE).
const ConnectionIDHeader = "X-Indri-Connection-Id"

// OriginChecker guards against cross-site hijacking. Allowed: requests with no
// Origin (CLI and most native clients), same-origin requests (React Native's
// iOS WebSocket always sends the target's own origin, and no other site can
// make a browser send it), and Origins in the comma-separated allowlist.
func OriginChecker(allowedOrigins string) func(*http.Request) bool {
	allowed := make(map[string]struct{})

	for _, o := range strings.Split(allowedOrigins, ",") {
		if o = strings.TrimSpace(o); o != "" {
			allowed[o] = struct{}{}
		}
	}

	return func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}

		if _, ok := allowed[origin]; ok {
			return true
		}

		u, err := url.Parse(origin)

		return err == nil && strings.EqualFold(u.Host, r.Host)
	}
}

// Route mounts h at method+path behind the origin policy, together with the
// OPTIONS preflight that browsers send before a cross-origin request carrying
// a custom header. Allowed browser origins get CORS headers so they can read
// the response; disallowed ones are refused before h runs.
func Route(mux *http.ServeMux, allowedOrigins, method, path string, h http.Handler) {
	allowed := OriginChecker(allowedOrigins)

	guard := func(w http.ResponseWriter, r *http.Request) bool {
		if !allowed(r) {
			http.Error(w, "origin not allowed", http.StatusForbidden)
			return false
		}

		if origin := r.Header.Get("Origin"); origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}

		return true
	}

	mux.HandleFunc(method+" "+path, func(w http.ResponseWriter, r *http.Request) {
		if guard(w, r) {
			h.ServeHTTP(w, r)
		}
	})

	mux.HandleFunc(http.MethodOptions+" "+path, func(w http.ResponseWriter, r *http.Request) {
		if guard(w, r) {
			w.Header().Set("Access-Control-Allow-Methods", method)
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, "+ConnectionIDHeader)
			w.WriteHeader(http.StatusNoContent)
		}
	})
}

// DebugRequested reports whether the request that opens a connection asked
// for debug mode (?debug=1), in which WriteEncoded sends JSON text instead of
// MessagePack. Transports must apply it before firing Connect.
func DebugRequested(r *http.Request) bool {
	return r != nil && r.URL.Query().Get("debug") == "1"
}
