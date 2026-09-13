package events

import (
	"bytes"
	"encoding/json"
	"log"
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
// the values of the remaining updates — a whole-object update for a newly
// added player, or the whole models.Stage that UpdateField publishes.
//
// The two halves need different work because they carry different things. A
// removed entry is a path and nothing else, so dropping private paths is the
// whole of its sanitization; an update carries a value as well, and a value has
// to be walked.
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

		stripped, err := stripKey(value, privateDataKey)
		if err != nil {
			// Fail closed. A value that cannot be rendered as JSON cannot be
			// shown to hold no private data, so it is dropped rather than
			// broadcast unexamined. Nothing is lost by doing so: the delta is
			// marshalled again on its way to the client, so a value that
			// fails here would have failed there, and no client would have
			// received this update anyway.
			log.Printf("dropping update to %q from the change delta: %v", path, err)

			continue
		}

		cleanUpdated[path] = stripped
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

// stripKey recursively removes the given key from any nested object, returning
// a new value: the caller's value is never modified, because the same value has
// already been stored and only the published copy may differ from it.
//
// Nearly every value arriving here is already the generic JSON view of a
// document — Diff walks ToMap's output, so its values are maps, slices and
// scalars keyed by json tag names, the same vocabulary the client holds.
// core.UpdateField is the exception: it publishes the raw Go value its caller
// handed it, and that value may be a struct, a pointer to one, or a map or
// slice of them. A struct is exactly where private data lives (models.Stage
// carries a PrivateData store, and so does every models.Scene inside it), so
// returning one untouched broadcast it to every player in the game.
//
// Such a value is therefore normalized through the JSON round trip this package
// already uses, and then stripped by the map and slice cases above. The
// alternative — walking the struct with reflection — would mean a second
// implementation of encoding/json's naming rules (json tags, omitempty,
// embedded and unexported fields, and types that marshal themselves, such as
// time.Time). A delta is *defined* by what json produces, so a second
// implementation that drifted from it by one field would be a leak. Normalizing
// keeps one definition of what the client sees, and leaves the published value
// in the same vocabulary as every value Diff produces.
//
// Normalizing changes the Go type of the published value, never its bytes on
// the wire: numbers keep their literal spelling (see toJSONValue), so an int64
// too large for a float64 still survives, and only the order of object keys can
// differ, which JSON does not define.
func stripKey(value interface{}, key string) (interface{}, error) {
	switch v := value.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(v))

		for k, val := range v {
			if k == key {
				continue
			}

			stripped, err := stripKey(val, key)
			if err != nil {
				return nil, err
			}

			out[k] = stripped
		}

		return out, nil
	case []interface{}:
		out := make([]interface{}, len(v))

		for i, item := range v {
			stripped, err := stripKey(item, key)
			if err != nil {
				return nil, err
			}

			out[i] = stripped
		}

		return out, nil
	default:
		// A scalar holds no fields to strip, so the common case costs no
		// marshalling.
		if !isComposite(value) {
			return value, nil
		}

		normalized, err := toJSONValue(value)
		if err != nil {
			return nil, err
		}

		// The recursion terminates: toJSONValue yields only maps, slices and
		// scalars, and a scalar is not composite.
		return stripKey(normalized, key)
	}
}

// isComposite reports whether a value may hold named fields that the map and
// slice cases of stripKey cannot reach on their own — a struct, or a map, slice
// or array that may contain one, through any number of pointers. A nil pointer
// holds nothing.
func isComposite(value interface{}) bool {
	v := reflect.ValueOf(value)
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return false
		}

		v = v.Elem()
	}

	switch v.Kind() {
	case reflect.Struct, reflect.Map, reflect.Slice, reflect.Array:
		return true
	default:
		return false
	}
}

// toJSONValue renders any value as its generic JSON representation. It is ToMap
// for a value that need not be an object: a struct becomes a
// map[string]interface{} keyed by json tag names, and a type that marshals
// itself (time.Time, primitive.ObjectID) becomes whatever it marshals to.
//
// Numbers are decoded as json.Number rather than float64, so re-marshalling the
// result reproduces the literal it was parsed from. Without that, normalizing a
// struct holding a large int64 — a nanosecond timestamp, a snowflake id — would
// silently round it, and the published value would no longer match the stored
// one.
func toJSONValue(value interface{}) (interface{}, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()

	var decoded interface{}
	if err := decoder.Decode(&decoded); err != nil {
		return nil, err
	}

	return decoded, nil
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
