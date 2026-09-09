// Package model holds hand-written GraphQL model types (the JSON scalar) that
// gqlgen binds to. gqlgen-generated models, if any, are written alongside.
package model

import (
	"encoding/json"
	"fmt"
	"io"
)

// JSON is the custom scalar backing dynamic/opaque payloads. It is raw JSON
// bytes so already-marshalled action responses and deltas pass through without
// re-encoding, and objects or arrays are both accepted.
type JSON json.RawMessage

// MarshalGQL writes the raw JSON bytes to the response.
func (j JSON) MarshalGQL(w io.Writer) {
	if len(j) == 0 {
		_, _ = w.Write([]byte("null"))
		return
	}

	_, _ = w.Write(j)
}

// UnmarshalGQL accepts a value parsed from GraphQL input and stores it as raw
// JSON bytes.
func (j *JSON) UnmarshalGQL(v interface{}) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("JSON scalar: %w", err)
	}

	*j = b

	return nil
}
