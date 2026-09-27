package events

import (
	"sort"
	"strconv"
	"strings"
)

// BuildPositionalMap walks obj and assigns each string key a 0-based integer
// index based on its sorted (case-sensitive, alphanumeric) position among its
// siblings. Array indices are not included — they remain raw integers.
func BuildPositionalMap(obj map[string]interface{}) map[string]int {
	result := make(map[string]int)
	buildPositionalMapInto("", obj, result)
	return result
}

func buildPositionalMapInto(prefix string, obj map[string]interface{}, result map[string]int) {
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for i, k := range keys {
		path := joinPath(prefix, k)
		result[path] = i

		if child, ok := obj[k].(map[string]interface{}); ok {
			buildPositionalMapInto(path, child, result)
		}
	}
}

// EncodePath converts a dotted string path (e.g. "stage.scenes.board.data.board.1.1")
// into a slice of path segments. Object-key segments are replaced with their
// positional integer index; array-index segments (numeric) are kept as raw ints.
func EncodePath(path string, schema map[string]interface{}, posMap map[string]int) []interface{} {
	segments := strings.Split(path, ".")
	result := make([]interface{}, 0, len(segments))
	var current interface{} = schema

	for i, seg := range segments {
		partialPath := strings.Join(segments[:i+1], ".")

		switch container := current.(type) {
		case map[string]interface{}:
			if idx, ok := posMap[partialPath]; ok {
				result = append(result, idx)
			} else {
				result = append(result, seg)
			}
			current = container[seg]
		default:
			if n, err := strconv.Atoi(seg); err == nil {
				result = append(result, n)
				if arr, ok := current.([]interface{}); ok && n < len(arr) {
					current = arr[n]
				} else {
					current = nil
				}
			} else {
				result = append(result, seg)
				current = nil
			}
		}
	}

	return result
}
