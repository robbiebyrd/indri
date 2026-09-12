// Package lua marshals values between Go and the embedded Lua VM.
//
// The Go side of this boundary is the *JSON domain* — exactly what
// events.ToMap produces: map[string]interface{}, []interface{}, string, bool,
// float64 and nil. That is deliberate. events.Diff compares before/after with
// reflect.DeepEqual over that representation, so a converter that handed back
// int64 for an integral number would make every unchanged field look changed.
//
// Every conversion is total or it fails. Nothing is ever silently dropped,
// truncated or coerced, because a value missing from a table returned by a
// script means "delete this field" — a truncated conversion would be
// indistinguishable from a deliberate deletion and would erase game state.
package lua

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

const (
	// defaultMaxDepth bounds how deeply a state document may nest. It exists to
	// stop unbounded recursion (a cyclic Go map, a pathologically nested Lua
	// table) from overflowing the Go stack.
	defaultMaxDepth = 32

	// defaultMaxNodes bounds how many values one conversion may produce. Every
	// scalar, table, array element and map entry counts as one node.
	defaultMaxNodes = 10000

	// maxSafeInteger is 2^53, the largest integer a float64 represents exactly.
	// events.ToMap round-trips through JSON, so every number becomes a float64;
	// an integer beyond this silently changes value on the way through. We
	// refuse it instead.
	maxSafeInteger = 1 << 53
)

// budget carries the depth and node limits, and the counters measured against
// them, through one conversion. It also holds the set of tables on the current
// descent path, which is how a cycle is detected.
//
// depth is incremented on the way into a table and decremented on the way out,
// so it measures the current path, not the deepest path seen. nodes only ever
// grows: it is the total size of the conversion.
type budget struct {
	depth    int
	nodes    int
	maxDepth int
	maxNodes int
	seen     map[*lua.LTable]struct{}
}

func newBudget(maxDepth, maxNodes int) *budget {
	return &budget{
		maxDepth: maxDepth,
		maxNodes: maxNodes,
		seen:     make(map[*lua.LTable]struct{}),
	}
}

func defaultBudget() *budget {
	return newBudget(defaultMaxDepth, defaultMaxNodes)
}

// countNode charges one node against the budget.
func (b *budget) countNode() error {
	if b.nodes++; b.nodes > b.maxNodes {
		return fmt.Errorf("state exceeds %d nodes", b.maxNodes)
	}

	return nil
}

// descend charges one level of nesting against the budget. The caller must
// always pair it with a deferred ascend, including on the error path, so the
// counter tracks the path and not the traversal.
func (b *budget) descend() error {
	if b.depth++; b.depth > b.maxDepth {
		return fmt.Errorf("state depth exceeds %d", b.maxDepth)
	}

	return nil
}

func (b *budget) ascend() {
	b.depth--
}

// toLua converts a JSON-domain Go value into a Lua value.
//
// It applies the default budget. A Go map can be cyclic; the depth cap is what
// stops such a value from recursing forever. In practice the only caller feeds
// it events.ToMap output, which came from JSON and therefore cannot be cyclic.
//
// A nil map value becomes lua.LNil, and assigning LNil to a table key is the
// same as not setting the key at all — Lua has no way to hold "present but
// null". That is the same identity the delta protocol already uses: an absent
// key and a null key both unmarshal to the Go zero value.
func toLua(L *lua.LState, v any) (lua.LValue, error) {
	return toLuaValue(L, v, defaultBudget())
}

func toLuaValue(L *lua.LState, v any, b *budget) (lua.LValue, error) {
	if err := b.countNode(); err != nil {
		return nil, err
	}

	switch val := v.(type) {
	case nil:
		return lua.LNil, nil
	case bool:
		return lua.LBool(val), nil
	case string:
		return lua.LString(val), nil
	case float64:
		return floatToLua(val)
	case float32:
		return floatToLua(float64(val))
	case int:
		return integerToLua(int64(val))
	case int64:
		return integerToLua(val)
	case map[string]interface{}:
		return mapToLua(L, val, b)
	case []interface{}:
		return sliceToLua(L, val, b)
	default:
		return nil, fmt.Errorf("cannot convert Go value of type %T to Lua", v)
	}
}

func mapToLua(L *lua.LState, m map[string]interface{}, b *budget) (lua.LValue, error) {
	if err := b.descend(); err != nil {
		return nil, err
	}

	defer b.ascend()

	tbl := L.NewTable()

	for key, val := range m {
		if err := validateKey(key); err != nil {
			return nil, err
		}

		converted, err := toLuaValue(L, val, b)
		if err != nil {
			return nil, fmt.Errorf("key %q: %w", key, err)
		}

		tbl.RawSetString(key, converted)
	}

	return tbl, nil
}

func sliceToLua(L *lua.LState, s []interface{}, b *budget) (lua.LValue, error) {
	if err := b.descend(); err != nil {
		return nil, err
	}

	defer b.ascend()

	tbl := L.NewTable()

	for i, val := range s {
		// A Lua array cannot hold a hole: storing nil at index i either shifts
		// the following elements or truncates the array. Both would corrupt the
		// document silently, so a null element is refused outright.
		if val == nil {
			return nil, fmt.Errorf("array index %d: nil cannot be represented inside a Lua array", i+1)
		}

		converted, err := toLuaValue(L, val, b)
		if err != nil {
			return nil, fmt.Errorf("array index %d: %w", i+1, err)
		}

		tbl.RawSetInt(i+1, converted)
	}

	return tbl, nil
}

func integerToLua(n int64) (lua.LValue, error) {
	if n > maxSafeInteger || n < -maxSafeInteger {
		return nil, fmt.Errorf("integer %d is outside the exactly representable range of +/-2^53", n)
	}

	return lua.LNumber(n), nil
}

func floatToLua(f float64) (lua.LValue, error) {
	if err := checkNumber(f); err != nil {
		return nil, err
	}

	return lua.LNumber(f), nil
}

// fromLua converts a Lua value into its JSON-domain Go equivalent, charging the
// work against b.
//
// Numbers always come back as float64, never int64: that is the type
// events.ToMap yields, and the delta path compares the two with
// reflect.DeepEqual.
func fromLua(v lua.LValue, b *budget) (any, error) {
	if err := b.countNode(); err != nil {
		return nil, err
	}

	switch val := v.(type) {
	case nil, *lua.LNilType:
		return nil, nil
	case lua.LBool:
		return bool(val), nil
	case lua.LString:
		return string(val), nil
	case lua.LNumber:
		return numberFromLua(val)
	case *lua.LTable:
		return tableFromLua(val, b)
	default:
		// Functions, userdata, threads and channels have no JSON form. They are
		// also how a script could smuggle a live reference into stored state.
		return nil, fmt.Errorf("cannot convert Lua value of type %s to Go", v.Type())
	}
}

// tableFromLua decides whether a table is an array or an object and converts it.
//
// The decision is made by LTable.Len() — the length of the dense 1..n prefix —
// together with the total number of entries. When they agree, the table is
// exactly 1..n with nothing in the hash part, and only then is it an array.
// ForEach is used to count, never to classify: it reports array indices and
// hash keys the same way, so it cannot tell the two parts apart on its own.
//
// Empty tables are objects. A Lua {} has Len() == 0 and an empty hash part, so
// it is genuinely ambiguous; "anything that is not a dense 1..n" is the same
// rule the rest of this function uses, matching gopher-json, and it keeps a Go
// empty map stable across any number of round trips. The cost is the mirror
// case: a Go empty *slice* comes back as an empty map, so an empty array does
// not survive a round trip. TestRoundTrip_EmptySliceBecomesEmptyObject pins
// that down.
func tableFromLua(tbl *lua.LTable, b *budget) (any, error) {
	if _, ok := b.seen[tbl]; ok {
		return nil, errors.New("cyclic table cannot be converted")
	}

	b.seen[tbl] = struct{}{}
	defer delete(b.seen, tbl)

	if err := b.descend(); err != nil {
		return nil, err
	}

	defer b.ascend()

	dense := tbl.Len()

	entries := 0
	tbl.ForEach(func(lua.LValue, lua.LValue) { entries++ })

	// The results are re-wrapped rather than returned directly: a nil
	// map[string]interface{} inside a non-nil any would make a failed
	// conversion test as non-nil for any caller checking the value.
	if dense > 0 && entries == dense {
		out, err := arrayFromLua(tbl, dense, b)
		if err != nil {
			return nil, err
		}

		return out, nil
	}

	out, err := objectFromLua(tbl, b)
	if err != nil {
		return nil, err
	}

	return out, nil
}

func arrayFromLua(tbl *lua.LTable, dense int, b *budget) ([]interface{}, error) {
	out := make([]interface{}, 0, dense)

	for i := 1; i <= dense; i++ {
		item, err := fromLua(tbl.RawGetInt(i), b)
		if err != nil {
			return nil, fmt.Errorf("array index %d: %w", i, err)
		}

		out = append(out, item)
	}

	return out, nil
}

func objectFromLua(tbl *lua.LTable, b *budget) (map[string]interface{}, error) {
	out := make(map[string]interface{})

	// ForEach cannot stop early, so the first failure is latched and the
	// remaining callbacks do nothing. The error is still returned, and out is
	// discarded — a partial object would read as a batch of deletions.
	var failure error

	tbl.ForEach(func(k, v lua.LValue) {
		if failure != nil {
			return
		}

		key, err := keyFromLua(k)
		if err != nil {
			failure = err

			return
		}

		item, err := fromLua(v, b)
		if err != nil {
			failure = fmt.Errorf("key %q: %w", key, err)

			return
		}

		out[key] = item
	})

	if failure != nil {
		return nil, failure
	}

	return out, nil
}

func numberFromLua(n lua.LNumber) (any, error) {
	f := float64(n)
	if err := checkNumber(f); err != nil {
		return nil, err
	}

	return f, nil
}

// checkNumber rejects the numbers that cannot survive the JSON representation
// the delta path uses: NaN and the infinities have no JSON form at all, and an
// integer beyond 2^53 changes value when it is parsed back.
func checkNumber(f float64) error {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return errors.New("NaN and infinity have no JSON representation")
	}

	if f == math.Trunc(f) && math.Abs(f) > maxSafeInteger {
		return fmt.Errorf("integer %.0f is outside the exactly representable range of +/-2^53", f)
	}

	return nil
}

// keyFromLua renders a Lua table key as an object field name.
//
// Integer keys are allowed and rendered in base ten: a table that mixes 1..n
// with string keys is an object, and its numeric keys still need names.
// Everything else — floats with a fraction, booleans, tables, functions — has
// no stable field name and is refused.
func keyFromLua(k lua.LValue) (string, error) {
	switch key := k.(type) {
	case lua.LString:
		name := string(key)
		if err := validateKey(name); err != nil {
			return "", err
		}

		return name, nil
	case lua.LNumber:
		f := float64(key)
		if math.IsNaN(f) || math.IsInf(f, 0) || f != math.Trunc(f) {
			return "", fmt.Errorf("table key %v is neither an integer nor a string", f)
		}

		if math.Abs(f) > maxSafeInteger {
			return "", fmt.Errorf("table key %.0f is outside the exactly representable range of +/-2^53", f)
		}

		return strconv.FormatInt(int64(f), 10), nil
	default:
		return "", fmt.Errorf("table key of type %s is not supported", k.Type())
	}
}

// validateKey refuses field names that are unsafe in the two systems a game
// document passes through.
//
// A dot is the delta protocol's path separator: events.joinPath builds a path
// as prefix + "." + key and SanitizeDelta splits it back on ".", so a key
// containing a dot forges a path segment. A key named "a.privateData" would
// produce the path "x.a.privateData", which SanitizeDelta would then redact —
// or, in the other direction, a key could impersonate a nested node and make
// the client apply an update to the wrong place.
//
// A dollar sign and a NUL are Mongo field-name violations; a leading dollar
// sign additionally reads as an update operator. An empty name is rejected
// because Mongo has no such field.
func validateKey(key string) error {
	switch {
	case key == "":
		return errors.New("empty table key is not a valid field name")
	case strings.Contains(key, "."):
		return fmt.Errorf("table key %q contains a dot, which would forge a delta path segment", key)
	case strings.Contains(key, "$"):
		return fmt.Errorf("table key %q contains a dollar sign, which is not a valid field name", key)
	case strings.ContainsRune(key, 0):
		return fmt.Errorf("table key %q contains a NUL, which is not a valid field name", key)
	default:
		return nil
	}
}
