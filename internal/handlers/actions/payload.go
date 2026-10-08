package actions

import "fmt"

// The size a script action's payload may be. A built-in action's payload is
// held to a list of named arguments by every transport; a script action's
// shape is known only to its script, so it is held to a size instead.
//
// They are the Lua conversion budget's own limits (internal/services/lua,
// defaultMaxDepth and defaultMaxNodes), counted the same way, so a payload
// accepted here is never refused by the conversion it goes through next.
// TestPayloadLimitsMatchTheConversionBudget holds the two together.
const (
	// MaxPayloadDepth is how deeply objects and lists may nest, the payload
	// itself counting as the first level.
	MaxPayloadDepth = 32

	// MaxPayloadValues is how many values the payload may hold in all: every
	// scalar, object and list, the payload itself included.
	MaxPayloadValues = 10000
)

// CheckScriptPayload refuses a payload deeper or bigger than a script action
// accepts. Its byte size is already bounded by the transport's body limit.
func CheckScriptPayload(payload map[string]interface{}) error {
	values := 0

	return checkValue(payload, 0, &values)
}

func checkValue(v interface{}, depth int, values *int) error {
	if *values++; *values > MaxPayloadValues {
		return fmt.Errorf("the payload holds more than %d values", MaxPayloadValues)
	}

	switch val := v.(type) {
	case map[string]interface{}:
		if depth+1 > MaxPayloadDepth {
			return fmt.Errorf("the payload nests deeper than %d levels", MaxPayloadDepth)
		}

		for _, item := range val {
			if err := checkValue(item, depth+1, values); err != nil {
				return err
			}
		}
	case []interface{}:
		if depth+1 > MaxPayloadDepth {
			return fmt.Errorf("the payload nests deeper than %d levels", MaxPayloadDepth)
		}

		for _, item := range val {
			if err := checkValue(item, depth+1, values); err != nil {
				return err
			}
		}
	}

	return nil
}
