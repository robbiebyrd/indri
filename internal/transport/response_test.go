package transport

import (
	"bytes"
	"log"
	"strconv"
	"strings"
	"testing"
)

// captureLog redirects the standard logger into a buffer for the duration of
// the test, so a report can be asserted instead of printed.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer

	out, flags := log.Writer(), log.Flags()

	log.SetOutput(&buf)
	log.SetFlags(0)

	t.Cleanup(func() {
		log.SetOutput(out)
		log.SetFlags(flags)
	})

	return &buf
}

// FirstResponse is what keeps a request/response transport from losing a
// handler's output in silence. Dropping is allowed and deliberate — one request
// answers with one document — but a drop nobody can see is indistinguishable
// from a handler that never produced anything, which is the failure this exists
// to prevent. So the assertions are two: the first response is always the one
// delivered, and every extra one is reported.
func TestFirstResponse(t *testing.T) {
	cases := map[string]struct {
		responses [][]byte
		want      string
		// wantDropped is how many responses the report must name. Zero means
		// nothing may be logged at all.
		wantDropped int
	}{
		"an action whose only effect is a broadcast returns nothing": {
			responses: nil,
			want:      "",
		},
		"the single response every built-in action returns today": {
			responses: [][]byte{[]byte(`{"ok":true}`)},
			want:      `{"ok":true}`,
		},
		"reconnect into an active game returns an auth payload and a keyframe": {
			responses: [][]byte{
				[]byte(`{"authenticated":true}`),
				[]byte(`{"id":"64f0","code":"room"}`),
			},
			want:        `{"authenticated":true}`,
			wantDropped: 1,
		},
		"a hook registered under received/processed adds another": {
			responses: [][]byte{
				[]byte(`{"first":true}`),
				[]byte(`{"second":true}`),
				[]byte(`{"third":true}`),
			},
			want:        `{"first":true}`,
			wantDropped: 2,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			logged := captureLog(t)

			got := FirstResponse("reconnect", tc.responses)

			if tc.want == "" {
				if got != nil {
					t.Errorf("FirstResponse() = %q, want nil so the caller can supply its own empty document", got)
				}
			} else if string(got) != tc.want {
				t.Errorf("FirstResponse() = %q, want %q", got, tc.want)
			}

			report := logged.String()

			if tc.wantDropped == 0 {
				if report != "" {
					t.Errorf("FirstResponse() logged %q, want nothing: no response was dropped", report)
				}

				return
			}

			if !strings.Contains(report, `"reconnect"`) {
				t.Errorf("report %q does not name the action, so a reader cannot tell which one lost output", report)
			}

			if !strings.Contains(report, strconv.Itoa(tc.wantDropped)) {
				t.Errorf("report %q does not say how many of the %d responses were dropped (%d)",
					report, len(tc.responses), tc.wantDropped)
			}
		})
	}
}
