package transport

import (
	"encoding/json"

	msgpack "github.com/vmihailenco/msgpack/v5"
)

// WriteEncoded writes payload as MessagePack by default, or JSON when the
// connection has the "debug" key set to true (set by ?debug=1 at upgrade).
func WriteEncoded(c Conn, payload interface{}) error {
	debug, _ := c.Get("debug")
	if v, ok := debug.(bool); ok && v {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		return c.Write(data)
	}
	data, err := msgpack.Marshal(payload)
	if err != nil {
		return err
	}
	return c.Write(data)
}
