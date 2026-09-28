package transport_test

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"
	"time"

	msgpack "github.com/vmihailenco/msgpack/v5"

	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/services/events"
	"github.com/robbiebyrd/indri/internal/transport"
)

// recordingConn captures what was written and whether it was binary.
type recordingConn struct {
	transport.Conn
	keys   map[string]any
	text   [][]byte
	binary [][]byte
}

func newRecordingConn() *recordingConn { return &recordingConn{keys: map[string]any{}} }

func (c *recordingConn) Get(key string) (any, bool) { v, ok := c.keys[key]; return v, ok }
func (c *recordingConn) Write(msg []byte) error     { c.text = append(c.text, msg); return nil }
func (c *recordingConn) WriteBinary(msg []byte) error {
	c.binary = append(c.binary, msg)
	return nil
}

// payloads are the shapes the server actually sends through WriteEncoded.
func payloads() map[string]any {
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)

	return map[string]any{
		"change event": events.ChangeEvent{
			ID: "g1", OperationType: events.OpUpdate, Timestamp: now, Collection: "game",
			UpdatedFields: [][]interface{}{{[]interface{}{1, 2}, "x"}},
			RemovedFields: []interface{}{[]interface{}{3}},
		},
		"keyframe": events.KeyframeWrapper{SV: "abc", Game: events.ClientView(mustMap(&models.Game{
			ID: "g1", Code: "C1", CreatedAt: now, UpdatedAt: now,
			Players:    map[string]models.Player{"p0": {Name: "Alice", UserID: "u1", Connected: true}},
			Teams:      map[string]models.Team{"red": {Name: "Red", PlayerIDs: []string{"p0"}}},
			PublicData: map[string]interface{}{"board": []interface{}{1, 2}},
		}))},
		"layout frame": events.LayoutFrame{O: events.OpLayout, V: "abc", Data: map[string]interface{}{"grid": 3}},
	}
}

// keyPaths lists every map key path in a decoded document, so two encodings
// can be compared by the names clients see, independent of value types.
func keyPaths(prefix string, v any) []string {
	var out []string

	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			p := prefix + "." + k
			out = append(out, p)
			out = append(out, keyPaths(p, child)...)
		}
	case []any:
		for _, child := range t {
			out = append(out, keyPaths(prefix+"[]", child)...)
		}
	}

	sort.Strings(out)

	return out
}

// TestWriteEncoded_MessagePackUsesTheJSONWireNames guards the contract the
// client relies on: a MessagePack payload decodes to the same keys as the
// JSON one. msgpack ignores json tags by default and would send Go field
// names ("OperationType", "PublicData"), which the client can't read.
func TestWriteEncoded_MessagePackUsesTheJSONWireNames(t *testing.T) {
	for name, payload := range payloads() {
		t.Run(name, func(t *testing.T) {
			c := newRecordingConn()

			if err := transport.WriteEncoded(c, payload); err != nil {
				t.Fatal(err)
			}
			if len(c.binary) != 1 || len(c.text) != 0 {
				t.Fatalf("MessagePack must go out as one binary frame; got %d binary, %d text", len(c.binary), len(c.text))
			}

			var fromMsgpack map[string]any
			if err := msgpack.Unmarshal(c.binary[0], &fromMsgpack); err != nil {
				t.Fatal(err)
			}

			jsonBytes, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			var fromJSON map[string]any
			if err := json.Unmarshal(jsonBytes, &fromJSON); err != nil {
				t.Fatal(err)
			}

			if got, want := keyPaths("", fromMsgpack), keyPaths("", fromJSON); !reflect.DeepEqual(got, want) {
				t.Fatalf("MessagePack keys differ from JSON wire names:\n msgpack: %v\n json:    %v", got, want)
			}
		})
	}
}

func TestWriteEncoded_DebugConnectionsGetJSONText(t *testing.T) {
	c := newRecordingConn()
	c.keys["debug"] = true

	if err := transport.WriteEncoded(c, events.LayoutFrame{O: events.OpLayout, V: "v1"}); err != nil {
		t.Fatal(err)
	}

	if len(c.text) != 1 || len(c.binary) != 0 {
		t.Fatalf("debug must be one text frame; got %d text, %d binary", len(c.text), len(c.binary))
	}
	if !json.Valid(c.text[0]) {
		t.Fatalf("debug frame is not JSON: %q", c.text[0])
	}
}

func mustMap(g *models.Game) map[string]interface{} {
	m, err := events.ToMap(g)
	if err != nil {
		panic(err)
	}

	return m
}
