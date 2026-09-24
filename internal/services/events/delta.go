package events

import (
	"encoding/json"
	"reflect"
	"strconv"
)

// Diff computes the dotted-path change between two documents (previously
// supplied by MongoDB's updateDescription). Nested objects and arrays are
// walked recursively so paths look like "players.<id>.host" or "board.1.1";
// scalars are compared whole. before/after are the JSON representations of
// the document.
func Diff(before, after map[string]interface{}) (updated map[string]interface{}, removed []string) {
	updated = make(map[string]interface{})
	diffInto("", before, after, updated, &removed)

	return updated, removed
}

func diffInto(prefix string, before, after map[string]interface{}, updated map[string]interface{}, removed *[]string) {
	for key, beforeVal := range before {
		path := joinPath(prefix, key)

		afterVal, ok := after[key]
		if !ok {
			*removed = append(*removed, path)
			continue
		}

		beforeMap, beforeIsMap := beforeVal.(map[string]interface{})
		afterMap, afterIsMap := afterVal.(map[string]interface{})
		beforeSlice, beforeIsSlice := beforeVal.([]interface{})
		afterSlice, afterIsSlice := afterVal.([]interface{})

		switch {
		case beforeIsMap && afterIsMap:
			diffInto(path, beforeMap, afterMap, updated, removed)
		case beforeIsSlice && afterIsSlice:
			diffSlice(path, beforeSlice, afterSlice, updated, removed)
		case !reflect.DeepEqual(beforeVal, afterVal):
			updated[path] = afterVal
		}
	}

	for key, afterVal := range after {
		if _, ok := before[key]; !ok {
			updated[joinPath(prefix, key)] = afterVal
		}
	}
}

func diffSlice(prefix string, before, after []interface{}, updated map[string]interface{}, removed *[]string) {
	minLen := len(before)
	if len(after) < minLen {
		minLen = len(after)
	}

	for i := 0; i < minLen; i++ {
		path := joinPath(prefix, strconv.Itoa(i))
		bMap, bIsMap := before[i].(map[string]interface{})
		aMap, aIsMap := after[i].(map[string]interface{})
		bSlice, bIsSlice := before[i].([]interface{})
		aSlice, aIsSlice := after[i].([]interface{})

		switch {
		case bIsMap && aIsMap:
			diffInto(path, bMap, aMap, updated, removed)
		case bIsSlice && aIsSlice:
			diffSlice(path, bSlice, aSlice, updated, removed)
		case !reflect.DeepEqual(before[i], after[i]):
			updated[path] = after[i]
		}
	}

	for i := minLen; i < len(before); i++ {
		*removed = append(*removed, joinPath(prefix, strconv.Itoa(i)))
	}
	for i := minLen; i < len(after); i++ {
		updated[joinPath(prefix, strconv.Itoa(i))] = after[i]
	}
}

const privateDataKey = "privateData"

var metadataKeys = map[string]bool{
	"updatedAt": true,
	"createdAt": true,
	"version":   true,
}

// SanitizeDelta removes private data and server-only metadata fields from a
// change delta so a broadcast delta has the same visibility as a sanitized
// keyframe.
func SanitizeDelta(updated [][]interface{}, removed []interface{}) ([][]interface{}, []interface{}) {
	cleanUpdated := make([][]interface{}, 0, len(updated))

	for _, pair := range updated {
		if len(pair) != 2 {
			continue
		}

		path, ok := pair[0].(string)
		if !ok || pathHasSegment(path, privateDataKey) || metadataKeys[path] {
			continue
		}

		cleanUpdated = append(cleanUpdated, []interface{}{path, stripKey(pair[1], privateDataKey)})
	}

	var cleanRemoved []interface{}

	for _, item := range removed {
		path, ok := item.(string)
		if ok && !pathHasSegment(path, privateDataKey) && !metadataKeys[path] {
			cleanRemoved = append(cleanRemoved, path)
		}
	}

	return cleanUpdated, cleanRemoved
}

func pathHasSegment(path, segment string) bool {
	start := 0

	for i := 0; i <= len(path); i++ {
		if i == len(path) || path[i] == '.' {
			if path[start:i] == segment {
				return true
			}

			start = i + 1
		}
	}

	return false
}

// stripKey recursively removes the given key from any nested object.
func stripKey(value interface{}, key string) interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(v))

		for k, val := range v {
			if k == key {
				continue
			}

			out[k] = stripKey(val, key)
		}

		return out
	case []interface{}:
		out := make([]interface{}, len(v))
		for i, item := range v {
			out[i] = stripKey(item, key)
		}

		return out
	default:
		return value
	}
}

func joinPath(prefix, key string) string {
	if prefix == "" {
		return key
	}

	return prefix + "." + key
}

// ToMap renders a value as its generic JSON object representation so Diff can
// walk it using the same field names (and json tags) the client sees.
func ToMap(v interface{}) (map[string]interface{}, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}

	return m, nil
}

// DiffDocuments computes the delta between two documents given as structs,
// using their JSON representation.
func DiffDocuments(before, after interface{}) (map[string]interface{}, []string, error) {
	beforeMap, err := ToMap(before)
	if err != nil {
		return nil, nil, err
	}

	afterMap, err := ToMap(after)
	if err != nil {
		return nil, nil, err
	}

	updated, removed := Diff(beforeMap, afterMap)

	return updated, removed, nil
}
