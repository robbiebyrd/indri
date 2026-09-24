package events_test

import (
	"reflect"
	"sort"
	"testing"

	"github.com/robbiebyrd/indri/internal/services/events"
)

func TestDiff_NestedFieldChange(t *testing.T) {
	before := map[string]interface{}{
		"players": map[string]interface{}{
			"p1": map[string]interface{}{"host": false, "name": "a"},
		},
	}
	after := map[string]interface{}{
		"players": map[string]interface{}{
			"p1": map[string]interface{}{"host": true, "name": "a"},
		},
	}

	updated, removed := events.Diff(before, after)

	if len(removed) != 0 {
		t.Errorf("expected no removals, got %v", removed)
	}

	want := map[string]interface{}{"players.p1.host": true}
	if !reflect.DeepEqual(updated, want) {
		t.Errorf("expected %v, got %v", want, updated)
	}
}

func TestDiff_AddedKey(t *testing.T) {
	before := map[string]interface{}{
		"players": map[string]interface{}{"p1": map[string]interface{}{"host": true}},
	}
	newPlayer := map[string]interface{}{"host": false}
	after := map[string]interface{}{
		"players": map[string]interface{}{
			"p1": map[string]interface{}{"host": true},
			"p2": newPlayer,
		},
	}

	updated, removed := events.Diff(before, after)

	if len(removed) != 0 {
		t.Errorf("expected no removals, got %v", removed)
	}

	if !reflect.DeepEqual(updated["players.p2"], newPlayer) {
		t.Errorf("expected new player at players.p2, got %v", updated)
	}
}

func TestDiff_RemovedKey(t *testing.T) {
	before := map[string]interface{}{
		"teams": map[string]interface{}{"t1": map[string]interface{}{}, "t2": map[string]interface{}{}},
	}
	after := map[string]interface{}{
		"teams": map[string]interface{}{"t2": map[string]interface{}{}},
	}

	updated, removed := events.Diff(before, after)

	if len(updated) != 0 {
		t.Errorf("expected no updates, got %v", updated)
	}

	if !reflect.DeepEqual(removed, []string{"teams.t1"}) {
		t.Errorf("expected [teams.t1] removed, got %v", removed)
	}
}

func TestDiff_ArrayReplacedWhole(t *testing.T) {
	before := map[string]interface{}{"board": []interface{}{"X", ""}}
	after := map[string]interface{}{"board": []interface{}{"X", "O"}}

	updated, _ := events.Diff(before, after)

	if !reflect.DeepEqual(updated["board"], []interface{}{"X", "O"}) {
		t.Errorf("expected whole board replaced, got %v", updated)
	}
}

func TestDiff_NoChange(t *testing.T) {
	m := map[string]interface{}{"a": 1, "b": map[string]interface{}{"c": 2}}

	updated, removed := events.Diff(m, m)

	if len(updated) != 0 || len(removed) != 0 {
		t.Errorf("expected no delta, got updated=%v removed=%v", updated, removed)
	}
}

func TestSanitizeDelta_PairArrayFormat(t *testing.T) {
	updated := [][]interface{}{
		{"players.p0.host", true},
		{"stage.privateData.secret", "x"},
	}
	removed := []interface{}{"players.p1", "teams.t1.privateData.key"}

	gotUpdated, gotRemoved := events.SanitizeDelta(updated, removed)

	if len(gotUpdated) != 1 || gotUpdated[0][0] != "players.p0.host" {
		t.Errorf("expected one public update, got %v", gotUpdated)
	}
	if len(gotRemoved) != 1 || gotRemoved[0] != "players.p1" {
		t.Errorf("expected one public removal, got %v", gotRemoved)
	}
}

func TestSanitizeDelta_DropsPrivatePaths(t *testing.T) {
	updated := [][]interface{}{
		{"players.p0.host", true},
		{"stage.privateData.answer", "42"},
		{"teams.t1.privateData", map[string]interface{}{"role": "spy"}},
	}
	removed := []interface{}{"players.p1", "stage.scenes.s1.privateData.key"}

	gotUpdated, gotRemoved := events.SanitizeDelta(updated, removed)

	if len(gotUpdated) != 1 || gotUpdated[0][0] != "players.p0.host" {
		t.Errorf("expected one public update, got %v", gotUpdated)
	}
	if len(gotRemoved) != 1 || gotRemoved[0] != "players.p1" {
		t.Errorf("wrong removals, got %v", gotRemoved)
	}
}

func TestSanitizeDelta_StripsNestedPrivateFromValue(t *testing.T) {
	// A whole-object update (e.g. a newly added player) must not carry its
	// nested privateData out on the wire.
	updated := [][]interface{}{
		{"players.p0", map[string]interface{}{
			"host":        true,
			"privateData": map[string]interface{}{"secret": "role"},
		}},
	}

	gotUpdated, _ := events.SanitizeDelta(updated, nil)

	player, ok := gotUpdated[0][1].(map[string]interface{})
	if !ok {
		t.Fatalf("expected player object, got %T", gotUpdated[0][1])
	}
	if _, ok := player["privateData"]; ok {
		t.Error("nested privateData leaked")
	}
	if player["host"] != true {
		t.Error("public field was stripped")
	}
}

func TestDiff_MultiplePaths(t *testing.T) {
	before := map[string]interface{}{"x": 1, "y": 2, "z": 3}
	after := map[string]interface{}{"x": 1, "y": 20, "w": 4}

	updated, removed := events.Diff(before, after)

	sort.Strings(removed)
	if !reflect.DeepEqual(removed, []string{"z"}) {
		t.Errorf("expected [z] removed, got %v", removed)
	}

	want := map[string]interface{}{"y": 20, "w": 4}
	if !reflect.DeepEqual(updated, want) {
		t.Errorf("expected %v, got %v", want, updated)
	}
}

func TestSanitizeDelta_MalformedPairsDropped(t *testing.T) {
	updated := [][]interface{}{
		{"valid.path", true},    // valid
		{"only-one-element"},    // length != 2 — should be dropped
		{42, "non-string path"}, // non-string path — should be dropped
		{"another.valid", "x"},  // valid
	}

	gotUpdated, _ := events.SanitizeDelta(updated, nil)

	if len(gotUpdated) != 2 {
		t.Errorf("expected 2 valid pairs, got %d: %v", len(gotUpdated), gotUpdated)
	}
	if gotUpdated[0][0] != "valid.path" {
		t.Errorf("first valid pair wrong: %v", gotUpdated[0])
	}
	if gotUpdated[1][0] != "another.valid" {
		t.Errorf("second valid pair wrong: %v", gotUpdated[1])
	}
}
