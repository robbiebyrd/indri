package events

import (
	"encoding/json"
	"reflect"
	"strings"
)

// Diff computes the dotted-path change between two documents (previously
// supplied by MongoDB's updateDescription). Nested objects are walked so paths
// look like "players.<id>.host"; arrays and scalars are treated as whole
// values. before/after are the JSON representations of the document.
//
// Document keys are arbitrary (player ids, team ids, script table keys), so a
// key may itself contain the separator. Keys are therefore escaped as they are
// joined — see escapeSegment — and every reader of a path must split it with
// splitPath rather than on a raw ".".
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
//
// Paths are decoded with splitPath, so the segment must be a real document key:
// a key merely named "x.privateData" is one segment, is not a privateData
// field, and is broadcast like any other key.
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
	for _, candidate := range splitPath(path) {
		if candidate == segment {
			return true
		}
	}

	return false
}

// splitPath decodes a dotted path back into its original keys. It is the
// inverse of joinPath: a backslash escapes the character after it, so only an
// unescaped "." separates segments. A path built without escaping (a key the
// caller wrote by hand, such as "stage.currentScene") contains no backslash and
// splits exactly as a plain strings.Split would, so both producers agree.
func splitPath(path string) []string {
	var (
		segments []string
		segment  strings.Builder
		escaped  bool
	)

	for i := 0; i < len(path); i++ {
		switch c := path[i]; {
		case escaped:
			segment.WriteByte(c)

			escaped = false
		case c == escapeChar:
			escaped = true
		case c == pathSeparator:
			segments = append(segments, segment.String())
			segment.Reset()
		default:
			segment.WriteByte(c)
		}
	}

	// A trailing lone backslash cannot have been produced by escapeSegment;
	// keep it literal rather than dropping a character from the key.
	if escaped {
		segment.WriteByte(escapeChar)
	}

	return append(segments, segment.String())
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

const (
	pathSeparator = '.'
	escapeChar    = '\\'
)

// joinPath appends one document key to an already-encoded path.
//
// The key is escaped first. Without that, a key holding a "." would forge a
// path segment: a widget id of "foo.privateData" would end a path in a segment
// literally equal to "privateData", so SanitizeDelta would drop every update to
// that widget, and the client would apply the update to a nested "privateData"
// child instead of to the key it was sent. Escaping keeps one key one segment
// whatever it contains.
func joinPath(prefix, key string) string {
	if prefix == "" {
		return escapeSegment(key)
	}

	return prefix + string(pathSeparator) + escapeSegment(key)
}

// escapeSegment encodes a single document key so it survives a round trip
// through a dotted path: a backslash escapes itself and the separator. Keys
// without either character — every id and json tag name the server generates —
// are returned unchanged, so the encoding is invisible in practice.
func escapeSegment(key string) string {
	if !strings.ContainsRune(key, pathSeparator) && !strings.ContainsRune(key, escapeChar) {
		return key
	}

	var escaped strings.Builder

	escaped.Grow(len(key) + 2)

	for i := 0; i < len(key); i++ {
		if c := key[i]; c == pathSeparator || c == escapeChar {
			escaped.WriteByte(escapeChar)
		}

		escaped.WriteByte(key[i])
	}

	return escaped.String()
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
