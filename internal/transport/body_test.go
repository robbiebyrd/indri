package transport

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// LimitBody is the one place the inbound size cap is decided, so it has to hold
// for both shapes a client can send: a declared Content-Length, which is
// refused outright, and a chunked body of unknown length, which can only be cut
// off as the wrapped handler reads it.
func TestLimitBody(t *testing.T) {
	cases := map[string]struct {
		size int
		// chunked drops the declared length, as a streaming client does.
		chunked bool
		// wantServed is whether the wrapped handler runs at all.
		wantServed bool
		wantRead   bool
		wantStatus int
	}{
		"empty body": {
			wantServed: true,
			wantRead:   true,
			wantStatus: http.StatusOK,
		},
		"inside the limit": {
			size:       MaxBodyBytes - 1,
			wantServed: true,
			wantRead:   true,
			wantStatus: http.StatusOK,
		},
		"exactly at the limit": {
			size:       MaxBodyBytes,
			wantServed: true,
			wantRead:   true,
			wantStatus: http.StatusOK,
		},
		"over the limit": {
			size:       MaxBodyBytes + 1,
			wantStatus: http.StatusRequestEntityTooLarge,
		},
		"over the limit without a declared length": {
			size:    MaxBodyBytes + 1,
			chunked: true,
			// Nothing announced the size, so the handler runs; the read is what
			// fails, and the handler is left to answer for it.
			wantServed: true,
			wantStatus: http.StatusOK,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var (
				served  bool
				readErr error
				read    int
			)

			handler := LimitBody(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				served = true

				body, err := io.ReadAll(r.Body)
				read, readErr = len(body), err
			}))

			req := httptest.NewRequest(
				http.MethodPost, "/graphql", strings.NewReader(strings.Repeat("a", tc.size)))
			if tc.chunked {
				req.ContentLength = -1
			}

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)

			if served != tc.wantServed {
				t.Errorf("handler served = %v, want %v", served, tc.wantServed)
			}

			if recorder.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", recorder.Code, tc.wantStatus)
			}

			if !tc.wantServed {
				return
			}

			if gotRead := readErr == nil; gotRead != tc.wantRead {
				t.Fatalf("reading the body = %v, want a read error: %v", readErr, !tc.wantRead)
			}

			if tc.wantRead && read != tc.size {
				t.Errorf("read %d bytes of the body, want %d", read, tc.size)
			}
		})
	}
}
