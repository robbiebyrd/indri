package events

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestClientView_StripsPrivateDataEverywhereAndTheLayout(t *testing.T) {
	doc := map[string]interface{}{
		"code":        "C1",
		"privateData": map[string]interface{}{"secret": 1},
		"data": map[string]interface{}{
			"layout": map[string]interface{}{"grid": 3},
			"score":  1,
		},
		"players": map[string]interface{}{
			"p0": map[string]interface{}{"name": "A", "privateData": map[string]interface{}{"hand": 1}},
		},
		"stage": map[string]interface{}{
			"privateData": map[string]interface{}{},
			"scenes": map[string]interface{}{
				"s1": map[string]interface{}{"privateData": map[string]interface{}{"x": 1}, "title": "t"},
			},
		},
		"board": []interface{}{map[string]interface{}{"privateData": 1, "v": 2}},
	}

	want := map[string]interface{}{
		"code":    "C1",
		"data":    map[string]interface{}{"score": 1},
		"players": map[string]interface{}{"p0": map[string]interface{}{"name": "A"}},
		"stage": map[string]interface{}{
			"scenes": map[string]interface{}{"s1": map[string]interface{}{"title": "t"}},
		},
		"board": []interface{}{map[string]interface{}{"v": 2}},
	}

	if got := ClientView(doc); !reflect.DeepEqual(got, want) {
		t.Fatalf("ClientView =\n %#v\nwant\n %#v", got, want)
	}

	if _, still := doc["privateData"]; !still {
		t.Fatal("ClientView modified its input")
	}
}

// Positions encoded against the client view must decode to the right key in
// the client's copy, even when private data and the layout sort earlier.
func TestEncodePath_AgainstClientViewMatchesTheClientSchema(t *testing.T) {
	full := map[string]interface{}{
		"code":        "C1",
		"data":        map[string]interface{}{"layout": 1, "turn": "x"},
		"privateData": map[string]interface{}{"k": 1},
		"stage":       map[string]interface{}{"currentScene": "board"},
	}

	view := ClientView(full)
	pos := BuildPositionalMap(view)

	for _, path := range []string{"stage.currentScene", "data.turn"} {
		encoded := EncodePath(path, view, pos)
		if got := resolveLikeTheClient(encoded, view); got != path {
			t.Errorf("%s encoded as %v decodes to %q on the client", path, encoded, got)
		}
	}
}

// resolveLikeTheClient mirrors client/services/positional-map.ts resolvePath.
func resolveLikeTheClient(path []interface{}, schema interface{}) string {
	var parts []string
	current := schema

	for _, seg := range path {
		idx, _ := seg.(int)
		obj, ok := current.(map[string]interface{})
		if !ok {
			return "unresolvable"
		}

		keys := make([]string, 0, len(obj))
		for k := range obj {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		if idx >= len(keys) {
			return "unresolvable"
		}
		parts = append(parts, keys[idx])
		current = obj[keys[idx]]
	}

	return strings.Join(parts, ".")
}
