package events

import (
	"reflect"
	"testing"
)

// A document key is arbitrary — a player id, a team id, a script table key — so
// it may contain the path separator. escapeSegment and splitPath must therefore
// be an exact round trip: whatever the key holds, it stays one segment.
func TestEscapeSegmentRoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		encoded string
	}{
		{name: "plain key is untouched", key: "players", encoded: "players"},
		{name: "object id is untouched", key: "65f1c2a4b8d3e9f0a1b2c3d4", encoded: "65f1c2a4b8d3e9f0a1b2c3d4"},
		{name: "empty key", key: "", encoded: ""},
		{name: "reserved key itself is not escaped", key: "privateData", encoded: "privateData"},
		{name: "forged private key", key: "foo.privateData", encoded: `foo\.privateData`},
		{name: "leading dot", key: ".privateData", encoded: `\.privateData`},
		{name: "several dots", key: "a.b.c", encoded: `a\.b\.c`},
		{name: "backslash", key: `a\b`, encoded: `a\\b`},
		{name: "trailing backslash", key: `a\`, encoded: `a\\`},
		{name: "backslash before a dot", key: `a\.b`, encoded: `a\\\.b`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := escapeSegment(tt.key); got != tt.encoded {
				t.Errorf("escapeSegment(%q) = %q, want %q", tt.key, got, tt.encoded)
			}

			// The key must survive being joined onto a path and split back
			// out, both on its own and with a neighbour on each side.
			path := joinPath(joinPath("", "root"), tt.key)
			path = joinPath(path, "leaf")

			want := []string{"root", tt.key, "leaf"}
			if got := splitPath(path); !reflect.DeepEqual(got, want) {
				t.Errorf("splitPath(%q) = %q, want %q", path, got, want)
			}
		})
	}
}

// Paths are also published by writers that build them by hand (UpdateField,
// playerConnectedKey). Those contain no backslash, so the decoder must treat
// them exactly as a plain split would — otherwise the two producers would
// disagree about the same path.
func TestSplitPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want []string
	}{
		{name: "hand-built path", path: "stage.currentScene", want: []string{"stage", "currentScene"}},
		{name: "single segment", path: "players", want: []string{"players"}},
		{name: "empty path", path: "", want: []string{""}},
		{name: "escaped separator stays one segment", path: `data.foo\.privateData`, want: []string{"data", "foo.privateData"}},
		{name: "escaped backslash", path: `data.a\\b`, want: []string{"data", `a\b`}},
		{name: "escaping an ordinary character is the character", path: `data.\privateData`, want: []string{"data", "privateData"}},
		{name: "trailing lone backslash is literal", path: `data.a\`, want: []string{"data", `a\`}},
		{name: "empty segments are preserved", path: "a..b", want: []string{"a", "", "b"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := splitPath(tt.path); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("splitPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
