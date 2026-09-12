package lua

import (
	"math"
	"reflect"
	"strings"
	"testing"

	lua "github.com/yuin/gopher-lua"
)

// newBareState builds an LState with no standard library. Marshalling never
// calls into Lua code, so the libraries are dead weight, and skipping them
// keeps these tests free of any sandbox concerns.
func newBareState(t *testing.T) *lua.LState {
	t.Helper()

	L := lua.NewState(lua.Options{SkipOpenLibs: true})
	t.Cleanup(L.Close)

	return L
}

// evalLua compiles and runs a Lua expression and returns its value. Tables
// built this way come from the real compiler, exercising the array/hash split
// exactly as a game script would produce it rather than as the Go API happens
// to lay it out.
func evalLua(t *testing.T, expr string) lua.LValue {
	t.Helper()

	L := lua.NewState()
	t.Cleanup(L.Close)

	if err := L.DoString("return " + expr); err != nil {
		t.Fatalf("evaluating %q: %v", expr, err)
	}

	v := L.Get(-1)
	L.Pop(1)

	return v
}

// TestRoundTrip_PreservesTheJSONDomain is the load-bearing test for the whole
// file: whatever a script is handed, it must be able to hand straight back
// unchanged. events.Diff compares before and after with reflect.DeepEqual, so
// any value that does not survive a round trip byte-for-byte would be published
// as a spurious change to every player in the game.
func TestRoundTrip_PreservesTheJSONDomain(t *testing.T) {
	tests := []struct {
		name  string
		value any
	}{
		{"nil", nil},
		{"true", true},
		{"false", false},
		{"string", "hello"},
		{"empty string", ""},
		{"zero", float64(0)},
		{"negative", float64(-17)},
		{"float keeps its fraction", 1.5},
		{"integer stays integral", float64(42)},
		{"largest exact integer", float64(maxSafeInteger)},
		{"smallest exact integer", float64(-maxSafeInteger)},
		{"empty map", map[string]interface{}{}},
		{"dense array", []interface{}{"a", "b", "c"}},
		{"array of numbers", []interface{}{float64(1), float64(2), float64(3)}},
		{"nested arrays", []interface{}{[]interface{}{float64(1)}, []interface{}{float64(2)}}},
		{
			"nested maps",
			map[string]interface{}{
				"stage": map[string]interface{}{
					"currentScene": "board",
					"scenes": map[string]interface{}{
						"board": map[string]interface{}{"data": map[string]interface{}{"turn": "x"}},
					},
				},
			},
		},
		{
			"game shaped document",
			map[string]interface{}{
				"code":    "ABCD",
				"private": false,
				"players": map[string]interface{}{
					"p1": map[string]interface{}{"score": float64(3), "host": true},
					"p2": map[string]interface{}{"score": float64(0), "host": false},
				},
				"teams": map[string]interface{}{
					"x": map[string]interface{}{
						"name":      "Crosses",
						"playerIds": []interface{}{"p1"},
						"data":      map[string]interface{}{"turn": true},
					},
				},
				"data": map[string]interface{}{"round": float64(2)},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			L := newBareState(t)

			lv, err := toLua(L, tt.value)
			if err != nil {
				t.Fatalf("toLua(%#v) failed: %v", tt.value, err)
			}

			got, err := fromLua(lv, defaultBudget())
			if err != nil {
				t.Fatalf("fromLua failed: %v", err)
			}

			if !reflect.DeepEqual(got, tt.value) {
				t.Errorf("round trip changed the value: want %#v, got %#v", tt.value, got)
			}
		})
	}
}

// TestRoundTrip_GoIntegersBecomeFloat64 pins the numeric domain. events.ToMap
// renders everything through JSON, so the delta path only ever sees float64. If
// fromLua returned int64 for an integral number, a state that a script never
// touched would compare unequal to the state it was built from.
func TestRoundTrip_GoIntegersBecomeFloat64(t *testing.T) {
	L := newBareState(t)

	lv, err := toLua(L, map[string]interface{}{"score": 7, "round": int64(2)})
	if err != nil {
		t.Fatalf("toLua failed: %v", err)
	}

	got, err := fromLua(lv, defaultBudget())
	if err != nil {
		t.Fatalf("fromLua failed: %v", err)
	}

	want := map[string]interface{}{"score": float64(7), "round": float64(2)}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("want %#v, got %#v", want, got)
	}
}

// TestFromLua_ClassifiesArrayVersusObject checks the rule that decides shape:
// LTable.Len() gives the dense 1..n prefix, and the table is an array only when
// the entry count agrees with it, meaning nothing sits in the hash part. Getting
// this wrong turns a board into an object or a lookup table into a list, and
// events.Diff replaces arrays whole while walking objects key by key — so the
// published delta changes shape too.
func TestFromLua_ClassifiesArrayVersusObject(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want any
	}{
		{"dense array is a slice", `{"a", "b"}`, []interface{}{"a", "b"}},
		{"single element array", `{"only"}`, []interface{}{"only"}},
		{"string keys are an object", `{a = 1}`, map[string]interface{}{"a": float64(1)}},
		{
			"mixed keys are an object",
			`{"a", "b", label = "z"}`,
			map[string]interface{}{"1": "a", "2": "b", "label": "z"},
		},
		{
			"sparse integer keys are an object",
			`{[1] = "a", [3] = "c"}`,
			map[string]interface{}{"1": "a", "3": "c"},
		},
		{
			"zero based keys are an object",
			`{[0] = "a", [1] = "b"}`,
			map[string]interface{}{"0": "a", "1": "b"},
		},
		{
			"negative keys are an object",
			`{[-1] = "a"}`,
			map[string]interface{}{"-1": "a"},
		},
		{"trailing nil is not an element", `{"a", "b", nil}`, []interface{}{"a", "b"}},
		{"empty table is an object", `{}`, map[string]interface{}{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := fromLua(evalLua(t, tt.expr), defaultBudget())
			if err != nil {
				t.Fatalf("fromLua(%s) failed: %v", tt.expr, err)
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("%s: want %#v (%T), got %#v (%T)", tt.expr, tt.want, tt.want, got, got)
			}
		})
	}
}

// TestFromLua_EmptyTableIsAnObject pins the empty-table decision so it cannot
// drift. A Lua {} has Len() == 0 and an empty hash part, so it is genuinely
// ambiguous between [] and {}. We always answer {} — the same "not a dense
// 1..n" rule everything else uses. What matters more than the choice is that it
// never changes: events.Diff compares with reflect.DeepEqual, and
// map[string]interface{}{} and []interface{}{} are not equal, so a converter
// that answered differently from one call to the next would publish a change
// for a field nobody touched.
func TestFromLua_EmptyTableIsAnObject(t *testing.T) {
	got, err := fromLua(evalLua(t, "{}"), defaultBudget())
	if err != nil {
		t.Fatalf("fromLua failed: %v", err)
	}

	if _, ok := got.(map[string]interface{}); !ok {
		t.Fatalf("an empty table must convert to a map, got %#v (%T)", got, got)
	}

	// Stability: converting the same empty table again gives an equal value.
	again, err := fromLua(evalLua(t, "{}"), defaultBudget())
	if err != nil {
		t.Fatalf("second fromLua failed: %v", err)
	}

	if !reflect.DeepEqual(got, again) {
		t.Errorf("empty-table conversion is not stable: %#v then %#v", got, again)
	}
}

// TestRoundTrip_EmptySliceBecomesEmptyObject records the one asymmetry the
// empty-table decision costs, so it is a known boundary rather than a surprise.
// Choosing {} for an empty table means an empty Go slice cannot come back as a
// slice: there is nothing in a bare Lua {} to say which it was. Callers that
// hand typed slices (models.Team.PlayerIDs, models.Stage.SceneOrder) across this
// boundary must cope with an empty one returning as an empty map.
func TestRoundTrip_EmptySliceBecomesEmptyObject(t *testing.T) {
	L := newBareState(t)

	lv, err := toLua(L, []interface{}{})
	if err != nil {
		t.Fatalf("toLua failed: %v", err)
	}

	got, err := fromLua(lv, defaultBudget())
	if err != nil {
		t.Fatalf("fromLua failed: %v", err)
	}

	if !reflect.DeepEqual(got, map[string]interface{}{}) {
		t.Errorf("want an empty map, got %#v (%T)", got, got)
	}
}

// TestFromLua_DepthCapErrors proves a document nested past the cap fails instead
// of coming back clipped. Absence in a returned table means deletion, so a
// truncated conversion would be applied as a delete of everything below the cut.
func TestFromLua_DepthCapErrors(t *testing.T) {
	deep := evalLua(t, `{a = {b = {c = {d = "too far"}}}}`)

	got, err := fromLua(deep, newBudget(2, defaultMaxNodes))
	if err == nil {
		t.Fatalf("expected a depth error, got %#v", got)
	}

	if got != nil {
		t.Errorf("a failed conversion must return nothing, got %#v", got)
	}

	if !strings.Contains(err.Error(), "depth") {
		t.Errorf("expected the error to name the depth cap, got %q", err)
	}
}

// TestFromLua_DepthCapAllowsExactlyTheCap checks the cap is a limit and not an
// off-by-one that rejects a document it should accept.
func TestFromLua_DepthCapAllowsExactlyTheCap(t *testing.T) {
	// Two levels of table: the outer object and the inner object.
	got, err := fromLua(evalLua(t, `{a = {b = "ok"}}`), newBudget(2, defaultMaxNodes))
	if err != nil {
		t.Fatalf("a document exactly at the cap must convert, got %v", err)
	}

	want := map[string]interface{}{"a": map[string]interface{}{"b": "ok"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("want %#v, got %#v", want, got)
	}
}

// TestFromLua_NodeCapErrors proves the size cap fails the whole conversion
// rather than returning the part that fit. A partial object is the dangerous
// case: every key that did not fit would be published as a removal.
func TestFromLua_NodeCapErrors(t *testing.T) {
	big := evalLua(t, `{"a", "b", "c", "d", "e", "f"}`)

	got, err := fromLua(big, newBudget(defaultMaxDepth, 3))
	if err == nil {
		t.Fatalf("expected a node error, got %#v", got)
	}

	if got != nil {
		t.Errorf("a failed conversion must return nothing, got %#v", got)
	}

	if !strings.Contains(err.Error(), "nodes") {
		t.Errorf("expected the error to name the node cap, got %q", err)
	}
}

// TestToLua_CapsError shows the caps guard the Go-to-Lua direction too, so an
// oversized state cannot be handed to a script half-built.
func TestToLua_CapsError(t *testing.T) {
	L := newBareState(t)

	tests := []struct {
		name   string
		budget *budget
		value  any
		want   string
	}{
		{
			name:   "depth",
			budget: newBudget(1, defaultMaxNodes),
			value:  map[string]interface{}{"a": map[string]interface{}{"b": "deep"}},
			want:   "depth",
		},
		{
			name:   "nodes",
			budget: newBudget(defaultMaxDepth, 2),
			value:  []interface{}{"a", "b", "c"},
			want:   "nodes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := toLuaValue(L, tt.value, tt.budget)
			if err == nil {
				t.Fatalf("expected a %s error, got %#v", tt.want, got)
			}

			if got != nil {
				t.Errorf("a failed conversion must return nothing, got %#v", got)
			}

			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("expected the error to name the %s cap, got %q", tt.want, err)
			}
		})
	}
}

// TestFromLua_CyclicTableRejected covers both shapes of cycle. Without this the
// converter recurses until the Go stack overflows, which kills the whole server
// process rather than one script invocation.
func TestFromLua_CyclicTableRejected(t *testing.T) {
	t.Run("self reference", func(t *testing.T) {
		L := newBareState(t)
		tbl := L.NewTable()
		tbl.RawSetString("self", tbl)

		if got, err := fromLua(tbl, defaultBudget()); err == nil {
			t.Fatalf("expected a cycle error, got %#v", got)
		}
	})

	t.Run("mutual reference", func(t *testing.T) {
		L := newBareState(t)
		a, b := L.NewTable(), L.NewTable()
		a.RawSetString("b", b)
		b.RawSetString("a", a)

		if got, err := fromLua(a, defaultBudget()); err == nil {
			t.Fatalf("expected a cycle error, got %#v", got)
		}
	})

	t.Run("a repeated table is not a cycle", func(t *testing.T) {
		L := newBareState(t)
		shared := L.NewTable()
		shared.RawSetString("k", lua.LString("v"))

		root := L.NewTable()
		root.RawSetString("first", shared)
		root.RawSetString("second", shared)

		got, err := fromLua(root, defaultBudget())
		if err != nil {
			t.Fatalf("a diamond is not a cycle and must convert, got %v", err)
		}

		want := map[string]interface{}{
			"first":  map[string]interface{}{"k": "v"},
			"second": map[string]interface{}{"k": "v"},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("want %#v, got %#v", want, got)
		}
	})
}

// TestFromLua_RejectsUnrepresentableValues covers the Lua types with no JSON
// form. Beyond being unserialisable, a function or a userdata is a live
// reference into the VM; storing one in game state would keep a dead LState
// alive and hand the next reader a value it cannot use.
func TestFromLua_RejectsUnrepresentableValues(t *testing.T) {
	L := newBareState(t)

	tests := []struct {
		name  string
		value lua.LValue
	}{
		{"function", L.NewFunction(func(*lua.LState) int { return 0 })},
		{"userdata", L.NewUserData()},
		{"channel", lua.LChannel(make(chan lua.LValue))},
		{"thread", L},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := fromLua(tt.value, defaultBudget()); err == nil {
				t.Fatalf("expected %s to be rejected, got %#v", tt.name, got)
			}
		})
	}
}

// TestFromLua_RejectsNestedUnrepresentableValues makes sure the check is not
// only at the top level — a function buried in the state is the realistic case.
func TestFromLua_RejectsNestedUnrepresentableValues(t *testing.T) {
	L := newBareState(t)
	tbl := L.NewTable()
	tbl.RawSetString("onMove", L.NewFunction(func(*lua.LState) int { return 0 }))

	got, err := fromLua(tbl, defaultBudget())
	if err == nil {
		t.Fatalf("expected a nested function to be rejected, got %#v", got)
	}

	if !strings.Contains(err.Error(), "onMove") {
		t.Errorf("expected the error to name the offending key, got %q", err)
	}
}

// TestFromLua_RejectsUnsafeKeys is the delta-path and Mongo safety boundary.
// events.joinPath builds a delta path as prefix + "." + key and SanitizeDelta
// splits it back on ".", so a key containing a dot forges a path segment: it can
// impersonate a nested node, or produce a path that the privateData redaction
// then silently swallows. A dollar sign and a NUL are Mongo field-name
// violations, and a leading dollar sign additionally reads as an update
// operator.
func TestFromLua_RejectsUnsafeKeys(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{"dot forges a path segment", "a.b"},
		{"dot impersonating privateData", "x.privateData"},
		{"leading dot", ".leading"},
		{"trailing dot", "trailing."},
		{"leading dollar reads as an operator", "$set"},
		{"embedded dollar", "a$b"},
		{"empty name", ""},
		{"NUL byte", "a\x00b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			L := newBareState(t)
			tbl := L.NewTable()
			tbl.RawSetString(tt.key, lua.LString("value"))

			got, err := fromLua(tbl, defaultBudget())
			if err == nil {
				t.Fatalf("expected key %q to be rejected, got %#v", tt.key, got)
			}
		})
	}
}

// TestFromLua_AcceptsOrdinaryKeys is the negative control for the rule above:
// the characters a real game uses in field names must still pass.
func TestFromLua_AcceptsOrdinaryKeys(t *testing.T) {
	L := newBareState(t)
	tbl := L.NewTable()

	keys := []string{"currentScene", "player_1", "player-2", "PlayerIDs", "1", "a b", "héllo"}
	for _, k := range keys {
		tbl.RawSetString(k, lua.LString("v"))
	}

	got, err := fromLua(tbl, defaultBudget())
	if err != nil {
		t.Fatalf("ordinary keys must be accepted, got %v", err)
	}

	object, ok := got.(map[string]interface{})
	if !ok {
		t.Fatalf("want a map, got %#v", got)
	}

	if len(object) != len(keys) {
		t.Errorf("want %d keys, got %d: %#v", len(keys), len(object), object)
	}
}

// TestToLua_RejectsUnsafeKeys applies the same rule in the other direction. A
// state that Go cannot safely publish must not reach a script either, otherwise
// the script hands it straight back and the failure surfaces at write time,
// where it is far harder to attribute.
func TestToLua_RejectsUnsafeKeys(t *testing.T) {
	for _, key := range []string{"a.b", "$set", "a$b", "", "a\x00b"} {
		t.Run(key, func(t *testing.T) {
			L := newBareState(t)

			got, err := toLua(L, map[string]interface{}{key: "value"})
			if err == nil {
				t.Fatalf("expected key %q to be rejected, got %#v", key, got)
			}
		})
	}
}

// TestFromLua_RejectsUnsupportedKeyTypes covers keys that have no stable field
// name at all. Rendering a float or a boolean as a name would invent a key the
// author never wrote.
func TestFromLua_RejectsUnsupportedKeyTypes(t *testing.T) {
	tests := []struct {
		name string
		key  lua.LValue
	}{
		{"fractional number", lua.LNumber(1.5)},
		{"boolean", lua.LTrue},
		{"integer beyond 2^53", lua.LNumber(float64(maxSafeInteger) * 4)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			L := newBareState(t)
			tbl := L.NewTable()
			tbl.RawSet(tt.key, lua.LString("value"))

			if got, err := fromLua(tbl, defaultBudget()); err == nil {
				t.Fatalf("expected key %v to be rejected, got %#v", tt.key, got)
			}
		})
	}
}

// TestFromLua_IntegerKeysBecomeFieldNames is the counterpart: a table that mixes
// 1..n with named keys is an object, and its numeric keys still need names. They
// are rendered in base ten, never in Go's float formatting, so index 1 is "1"
// and not "1e+00".
func TestFromLua_IntegerKeysBecomeFieldNames(t *testing.T) {
	got, err := fromLua(evalLua(t, `{"first", "second", named = true}`), defaultBudget())
	if err != nil {
		t.Fatalf("fromLua failed: %v", err)
	}

	want := map[string]interface{}{"1": "first", "2": "second", "named": true}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("want %#v, got %#v", want, got)
	}
}

// TestFromLua_RejectsIntegersBeyond2Pow53 protects the numeric contract.
// events.ToMap round-trips through JSON, so every number lands as a float64; an
// integer larger than 2^53 comes back as a different number and nothing reports
// it. Rejecting at the boundary is the only place the loss is still visible.
func TestFromLua_RejectsIntegersBeyond2Pow53(t *testing.T) {
	tests := []struct {
		name    string
		number  float64
		wantErr bool
	}{
		{"exactly 2^53 is exact", float64(maxSafeInteger), false},
		{"exactly -2^53 is exact", -float64(maxSafeInteger), false},
		{"just inside the range", float64(maxSafeInteger) - 1, false},
		{"beyond 2^53", float64(maxSafeInteger) * 2, true},
		{"beyond -2^53", -float64(maxSafeInteger) * 2, true},
		{"a fractional number of any size is fine", 1.5, false},
		{"NaN has no JSON form", math.NaN(), true},
		{"positive infinity has no JSON form", math.Inf(1), true},
		{"negative infinity has no JSON form", math.Inf(-1), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := fromLua(lua.LNumber(tt.number), defaultBudget())

			switch {
			case tt.wantErr && err == nil:
				t.Fatalf("expected %v to be rejected, got %#v", tt.number, got)
			case !tt.wantErr && err != nil:
				t.Fatalf("expected %v to be accepted, got %v", tt.number, err)
			}
		})
	}
}

// TestToLua_RejectsIntegersBeyond2Pow53 mirrors the rule for Go integers, which
// really can hold a value a float64 cannot.
func TestToLua_RejectsIntegersBeyond2Pow53(t *testing.T) {
	tests := []struct {
		name    string
		value   any
		wantErr bool
	}{
		{"exactly 2^53", int64(maxSafeInteger), false},
		{"exactly -2^53", int64(-maxSafeInteger), false},
		{"beyond 2^53", int64(maxSafeInteger) + 1, true},
		{"beyond -2^53", int64(-maxSafeInteger) - 1, true},
		{"ordinary int", 42, false},
		{"NaN", math.NaN(), true},
		{"infinity", math.Inf(1), true},
		{"float beyond 2^53", float64(maxSafeInteger) * 2, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			L := newBareState(t)

			got, err := toLua(L, tt.value)

			switch {
			case tt.wantErr && err == nil:
				t.Fatalf("expected %#v to be rejected, got %v", tt.value, got)
			case !tt.wantErr && err != nil:
				t.Fatalf("expected %#v to be accepted, got %v", tt.value, err)
			}
		})
	}
}

// TestToLua_RejectsUnsupportedGoTypes keeps the Go side of the boundary as
// narrow as the Lua side. Only the JSON domain crosses; anything else is a
// caller bug and must be reported as one instead of arriving in Lua as nil.
func TestToLua_RejectsUnsupportedGoTypes(t *testing.T) {
	type point struct{ X int }

	tests := []struct {
		name  string
		value any
	}{
		{"channel", make(chan int)},
		{"function", func() {}},
		{"struct", point{X: 1}},
		{"typed map", map[string]string{"a": "b"}},
		{"typed slice", []string{"a"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			L := newBareState(t)

			if got, err := toLua(L, tt.value); err == nil {
				t.Fatalf("expected %T to be rejected, got %v", tt.value, got)
			}
		})
	}
}

// TestToLua_RejectsNilInsideAnArray is a truncation guard. Lua arrays cannot
// hold a hole: writing nil at an index either shifts the following elements up
// or cuts the array short. Both silently change the document, so a null element
// is refused rather than quietly reshaping the array.
func TestToLua_RejectsNilInsideAnArray(t *testing.T) {
	L := newBareState(t)

	got, err := toLua(L, []interface{}{"a", nil, "c"})
	if err == nil {
		t.Fatalf("expected a nil array element to be rejected, got %v", got)
	}

	if !strings.Contains(err.Error(), "array index 2") {
		t.Errorf("expected the error to name the offending index, got %q", err)
	}
}

// TestToLua_NilMapValueIsAbsentInLua documents the one lossy case that Lua
// itself imposes: a table has no way to hold "present but null", so assigning
// nil to a key is the same as never setting it. This matches the delta
// protocol, where an absent key and a null key both unmarshal to the Go zero
// value.
func TestToLua_NilMapValueIsAbsentInLua(t *testing.T) {
	L := newBareState(t)

	lv, err := toLua(L, map[string]interface{}{"present": "yes", "absent": nil})
	if err != nil {
		t.Fatalf("toLua failed: %v", err)
	}

	tbl, ok := lv.(*lua.LTable)
	if !ok {
		t.Fatalf("want a table, got %T", lv)
	}

	if tbl.RawGetString("absent") != lua.LNil {
		t.Errorf("a nil map value must be absent in Lua, got %v", tbl.RawGetString("absent"))
	}

	got, err := fromLua(tbl, defaultBudget())
	if err != nil {
		t.Fatalf("fromLua failed: %v", err)
	}

	want := map[string]interface{}{"present": "yes"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("want %#v, got %#v", want, got)
	}
}

// TestToLua_CyclicGoMapIsRejected proves the depth cap backstops the Go
// direction. A Go map really can point at itself, and without a bound the
// converter would recurse until the process stack overflows, taking the whole
// server with it rather than failing one invocation.
func TestToLua_CyclicGoMapIsRejected(t *testing.T) {
	L := newBareState(t)

	m := map[string]interface{}{}
	m["self"] = m

	got, err := toLua(L, m)
	if err == nil {
		t.Fatalf("expected a cyclic Go map to be rejected, got %v", got)
	}

	if !strings.Contains(err.Error(), "depth") {
		t.Errorf("expected the depth cap to catch the cycle, got %q", err)
	}
}

// TestBudget_DepthUnwindsOnTheErrorPath guards a subtle bookkeeping bug: depth
// measures the current path, so it must be decremented even when a sibling
// conversion fails. If it leaked, a state with many shallow branches would
// eventually be rejected for a depth it never reached.
func TestBudget_DepthUnwindsOnTheErrorPath(t *testing.T) {
	L := newBareState(t)
	b := newBudget(defaultMaxDepth, defaultMaxNodes)

	// A key that fails validation aborts partway through a nested conversion.
	if _, err := toLuaValue(L, map[string]interface{}{"a": map[string]interface{}{"b.c": 1}}, b); err == nil {
		t.Fatal("expected the unsafe nested key to be rejected")
	}

	if b.depth != 0 {
		t.Errorf("depth must unwind to zero after a failed conversion, got %d", b.depth)
	}
}

// TestFromLua_CycleSetUnwindsOnSuccess guards the same bookkeeping for cycle
// detection. If a table stayed in the seen set after it was converted, the next
// legitimate reference to it would be misreported as a cycle.
func TestFromLua_CycleSetUnwindsOnSuccess(t *testing.T) {
	L := newBareState(t)
	b := newBudget(defaultMaxDepth, defaultMaxNodes)

	tbl := L.NewTable()
	tbl.RawSetString("k", lua.LString("v"))

	if _, err := fromLua(tbl, b); err != nil {
		t.Fatalf("fromLua failed: %v", err)
	}

	if len(b.seen) != 0 {
		t.Errorf("the cycle set must be empty after a conversion, got %d entries", len(b.seen))
	}

	if _, err := fromLua(tbl, b); err != nil {
		t.Fatalf("converting the same table again must succeed, got %v", err)
	}
}
