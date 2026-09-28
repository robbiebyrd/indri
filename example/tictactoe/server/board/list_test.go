package board

import (
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Stores decode stored arrays as different slice types (MongoDB: bson.A;
// the others: []any); game code must treat them alike.
func TestList(t *testing.T) {
	want := []any{"X", ""}

	for name, v := range map[string]any{
		"[]any":    []any{"X", ""},
		"bson.A":   bson.A{"X", ""},
		"[]string": []string{"X", ""},
	} {
		got, ok := List(v)
		if !ok || !reflect.DeepEqual(got, want) {
			t.Errorf("%s: List = %v, %v", name, got, ok)
		}
	}

	for name, v := range map[string]any{"nil": nil, "string": "X", "map": map[string]any{}} {
		if _, ok := List(v); ok {
			t.Errorf("%s: List reported a list", name)
		}
	}
}
