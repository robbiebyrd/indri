package transport

import (
	"net/http"
	"sort"
	"strings"

	"github.com/robbiebyrd/indri/internal/repo/env"
)

// preflightMethods and preflightHeaders are what a browser is told the real
// request may use. They cover every transport: GET opens an SSE stream, POST
// carries an action or a GraphQL operation, and the session bearer token
// travels in Authorization.
const (
	preflightMethods = "GET, POST, OPTIONS"
	preflightHeaders = "Authorization, Content-Type"
)

// OriginPolicy decides which browser origins may talk to the server, and
// applies the CORS headers that let an allowed one through. Every transport
// shares one policy, so a browser that can open a WebSocket can also open an
// SSE stream and POST an action.
//
// Requests with no Origin (native and CLI clients) are always allowed. Beyond
// that, INDRI_ALLOWED_ORIGINS is the whole policy:
//
//   - Empty (the default): closed. No browser origin is allowed, the server's
//     own included. An open default would let any page a visitor opens reach
//     a server on their localhost or private network through their browser.
//   - A list: exactly its origins. The server's own origin gets no pass, since
//     a DNS-rebinding page presents exactly that; a React Native iOS client,
//     which sends it, must be listed.
//   - "*": open. Every origin is allowed. Connections carry no ambient
//     credentials (no cookies; a client authenticates with login or a token
//     it holds), so a foreign page cannot act as a player — but the
//     localhost/LAN reach above applies, which is why open is an opt-in and
//     boot warns about it. "*" anywhere in the list wins over the rest.
//
// A nil *OriginPolicy is a transport built without one — a wiring bug, not a
// configuration — and denies every cross-origin request.
type OriginPolicy struct {
	allowed map[string]struct{}
	open    bool
}

// openEntry is the INDRI_ALLOWED_ORIGINS entry that opts into open mode.
const openEntry = "*"

func NewOriginPolicy(allowedOrigins string) *OriginPolicy {
	allowed := make(map[string]struct{})

	policy := &OriginPolicy{allowed: allowed}

	for _, o := range strings.Split(allowedOrigins, ",") {
		switch o = strings.TrimSpace(o); o {
		case "":
		case openEntry:
			policy.open = true
		default:
			allowed[o] = struct{}{}
		}
	}

	return policy
}

// Open reports whether every browser origin is allowed ("*").
func (p *OriginPolicy) Open() bool {
	return p != nil && p.open
}

// String names the policy in force, for the boot log.
func (p *OriginPolicy) String() string {
	switch {
	case p.Open():
		return `open: every browser origin is allowed (INDRI_ALLOWED_ORIGINS="*")`
	case p == nil || len(p.allowed) == 0:
		return "closed: no browser origin is allowed; set INDRI_ALLOWED_ORIGINS to list some"
	}

	origins := make([]string, 0, len(p.allowed))
	for o := range p.allowed {
		origins = append(origins, o)
	}

	sort.Strings(origins)

	return "allowlist: " + strings.Join(origins, ", ")
}

// OriginPolicyFromEnv builds the policy from INDRI_ALLOWED_ORIGINS, so every
// transport enforces one allowlist from one place.
func OriginPolicyFromEnv() *OriginPolicy {
	return NewOriginPolicy(env.GetEnv().AllowedOrigins)
}

// Allows reports whether r may proceed. A request with no Origin is allowed:
// that is a native or CLI client, which is not subject to the browser's
// ambient-credential problem CORS exists to contain.
func (p *OriginPolicy) Allows(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}

	if p == nil {
		return false
	}

	if p.open {
		return true
	}

	_, ok := p.allowed[origin]

	return ok
}

// Middleware rejects disallowed origins, answers CORS preflights, and adds the
// response headers an allowed cross-origin request needs. Wrap every
// browser-reachable HTTP route with it.
func (p *OriginPolicy) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The response varies by Origin whatever the outcome. Without this, a
		// shared cache can hand one origin's CORS headers to a browser on
		// another origin.
		w.Header().Set("Vary", "Origin")

		if !p.Allows(r) {
			http.Error(w, "origin not allowed", http.StatusForbidden)

			return
		}

		origin := r.Header.Get("Origin")
		if origin == "" {
			// A non-browser client: no CORS headers are meaningful.
			next.ServeHTTP(w, r)

			return
		}

		header := w.Header()
		header.Set("Access-Control-Allow-Origin", origin)

		if isPreflight(r) {
			header.Set("Access-Control-Allow-Methods", preflightMethods)
			header.Set("Access-Control-Allow-Headers", preflightHeaders)
			w.WriteHeader(http.StatusNoContent)

			return
		}

		next.ServeHTTP(w, r)
	})
}

// isPreflight reports whether r is a CORS preflight rather than a real request.
func isPreflight(r *http.Request) bool {
	return r.Method == http.MethodOptions &&
		r.Header.Get("Access-Control-Request-Method") != ""
}
