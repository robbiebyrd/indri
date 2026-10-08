package actions

import (
	"strconv"
	"strings"
	"testing"
)

func nested(depth int) map[string]interface{} {
	payload := map[string]interface{}{}
	for range depth - 1 {
		payload = map[string]interface{}{"inner": payload}
	}

	return payload
}

func wide(values int) map[string]interface{} {
	payload := make(map[string]interface{}, values)
	for i := range values {
		payload["k"+strconv.Itoa(i)] = i
	}

	return payload
}

// A script action's payload has a shape only its script knows, so it cannot be
// held to a list of named arguments the way a built-in's is. It is held to a
// size instead: the size the Lua conversion it is about to go through accepts.
func TestCheckScriptPayload(t *testing.T) {
	cases := map[string]struct {
		payload map[string]interface{}
		refused string
	}{
		"an ordinary move":          {payload: map[string]interface{}{"row": 1.0, "col": 2.0}},
		"none at all":               {payload: nil},
		"nested to the limit":       {payload: nested(MaxPayloadDepth)},
		"nested past the limit":     {payload: nested(MaxPayloadDepth + 1), refused: "deep"},
		"as many values as allowed": {payload: wide(MaxPayloadValues - 1)},
		"more values than allowed":  {payload: wide(MaxPayloadValues), refused: "values"},
		"a list past the limit": {
			payload: map[string]interface{}{"list": make([]interface{}, MaxPayloadValues)},
			refused: "values",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := CheckScriptPayload(tc.payload)

			switch {
			case tc.refused == "" && err != nil:
				t.Errorf("refused: %v", err)
			case tc.refused != "" && err == nil:
				t.Error("accepted")
			case tc.refused != "" && !strings.Contains(err.Error(), tc.refused):
				t.Errorf("refused with %q, want it to say why (%q)", err, tc.refused)
			}
		})
	}
}
