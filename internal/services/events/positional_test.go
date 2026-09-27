package events_test

import (
	"testing"

	"github.com/robbiebyrd/indri/internal/services/events"
)

func TestBuildPositionalMap_SortsKeys(t *testing.T) {
	obj := map[string]interface{}{
		"stage":   map[string]interface{}{},
		"players": map[string]interface{}{},
		"code":    "X",
		"data":    map[string]interface{}{},
	}
	posMap := events.BuildPositionalMap(obj)

	// Sorted: code=0, data=1, players=2, stage=3
	if posMap["code"] != 0 || posMap["data"] != 1 || posMap["players"] != 2 || posMap["stage"] != 3 {
		t.Errorf("unexpected positions: %v", posMap)
	}
}

func TestEncodePath_ObjectSegments(t *testing.T) {
	schema := map[string]interface{}{
		"stage": map[string]interface{}{
			"scenes": map[string]interface{}{
				"board": map[string]interface{}{
					"data": map[string]interface{}{
						"board": []interface{}{},
					},
				},
			},
		},
	}
	posMap := events.BuildPositionalMap(schema)

	path := events.EncodePath("stage.scenes.board.data.board.1.1", schema, posMap)

	// stage→0 (only root key), scenes→0, board→0, data→0, board→0, then raw indices 1,1
	if len(path) != 7 {
		t.Errorf("expected 7 segments, got %d: %v", len(path), path)
	}
	want := []interface{}{0, 0, 0, 0, 0, 1, 1}
	for i, seg := range want {
		if path[i] != seg {
			t.Errorf("segment %d: expected %v, got %v (full path: %v)", i, seg, path[i], path)
		}
	}
}

func TestEncodePath_RoundTrip(t *testing.T) {
	schema := map[string]interface{}{
		"a": map[string]interface{}{
			"b": "value",
		},
	}
	posMap := events.BuildPositionalMap(schema)
	path := events.EncodePath("a.b", schema, posMap)

	if len(path) != 2 {
		t.Errorf("expected 2 segments, got %v", path)
	}
	// a is the only root key → index 0. b is the only child key → index 0.
	if path[0] != 0 || path[1] != 0 {
		t.Errorf("expected [0, 0], got %v", path)
	}
}

func TestEncodePath_UnknownKeyFallback(t *testing.T) {
	schema := map[string]interface{}{
		"a": map[string]interface{}{},
	}
	posMap := events.BuildPositionalMap(schema)

	// "a.unknown" — "unknown" is not in schema, so falls back to raw string.
	path := events.EncodePath("a.unknown", schema, posMap)

	if len(path) != 2 {
		t.Errorf("expected 2 segments, got %d: %v", len(path), path)
	}
	if path[0] != 0 {
		t.Errorf("expected segment 0 to be 0, got %v", path[0])
	}
	if path[1] != "unknown" {
		t.Errorf("expected segment 1 to be 'unknown' (fallback), got %v", path[1])
	}
}

func TestSanitizeDelta_PassesThroughEncodedPaths(t *testing.T) {
	encodedPath := []interface{}{0, 2, 3}
	updated := [][]interface{}{
		{encodedPath, "value"},
	}
	gotUpdated, _ := events.SanitizeDelta(updated, nil)
	if len(gotUpdated) != 1 {
		t.Errorf("expected encoded path to pass through, got %v", gotUpdated)
	}
	if got, ok := gotUpdated[0][0].([]interface{}); !ok || len(got) != 3 {
		t.Errorf("encoded path not preserved: %v", gotUpdated[0][0])
	}
}
