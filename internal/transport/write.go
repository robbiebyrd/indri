package transport

import (
	"bytes"
	"encoding/json"

	msgpack "github.com/vmihailenco/msgpack/v5"
)

// WriteEncoded writes payload as MessagePack by default, or as JSON text when
// the connection has the "debug" key set to true (?debug=1 when connecting).
func WriteEncoded(c Conn, payload interface{}) error {
	debug, _ := c.Get("debug")
	if v, ok := debug.(bool); ok && v {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		return c.Write(data)
	}

	data, err := encodeMessagePack(payload)
	if err != nil {
		return err
	}

	return c.WriteBinary(data)
}

// encodeMessagePack encodes with the json struct tags, so MessagePack uses
// the same field names as JSON and honors json:"-" and omitempty. By default
// msgpack would use Go field names, which clients can't read.
func encodeMessagePack(payload interface{}) ([]byte, error) {
	var buf bytes.Buffer

	enc := msgpack.NewEncoder(&buf)
	enc.SetCustomStructTag("json")

	if err := enc.Encode(payload); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
