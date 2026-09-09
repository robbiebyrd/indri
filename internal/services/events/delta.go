package events

import (
	"encoding/json"
	"reflect"
)

// Diff computes the dotted-path change between two documents (previously
// supplied by MongoDB's updateDescription). Nested objects are walked so paths
// look like "players.<id>.host"; arrays and scalars are treated as whole
// values. before/after are the JSON representations of the document.
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

		switch {
		case beforeIsMap && afterIsMap:
			diffInto(path, beforeMap, afterMap, updated, removed)
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

const privateDataKey = "privateData"

// SanitizeDelta removes private data from a change delta so a broadcast delta
// has the same visibility as a sanitized keyframe. Any path that refers to a
// privateData field is dropped, and privateData is stripped recursively from
// the values of the remaining updates (e.g. a whole-object update for a newly
// added player).
func SanitizeDelta(updated map[string]interface{}, removed []string) (map[string]interface{}, []string) {
	cleanUpdated := make(map[string]interface{}, len(updated))

	for path, value := range updated {
		if pathHasSegment(path, privateDataKey) {
			continue
		}

		cleanUpdated[path] = stripKey(value, privateDataKey)
	}

	var cleanRemoved []string

	for _, path := range removed {
		if pathHasSegment(path, privateDataKey) {
			continue
		}

		cleanRemoved = append(cleanRemoved, path)
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
