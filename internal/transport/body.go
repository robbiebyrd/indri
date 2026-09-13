package transport

import "net/http"

// MaxBodyBytes caps an inbound request body, for every transport that reads
// one. Action arguments are a handful of short strings, and even a layout op
// carrying a script source or an opaque config stays well inside this;
// anything larger is a mistake or an attack.
const MaxBodyBytes = 64 << 10

// LimitBody caps the request body h may read. A request that declares more
// than MaxBodyBytes is refused with 413 without reading any of it; one that
// declares nothing (a chunked body) is cut off at the same limit, and h sees
// the read fail.
//
// It hands w on unwrapped, so a WebSocket upgrade served by the same handler
// still hijacks the connection. An upgrade carries no body, so the limit never
// applies to one.
func LimitBody(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ContentLength > MaxBodyBytes {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)

			return
		}

		limited := *r
		limited.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)

		h.ServeHTTP(w, &limited)
	})
}
