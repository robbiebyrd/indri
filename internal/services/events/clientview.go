package events

// ClientView returns the part of a game document (its JSON map form) that
// clients hold: private data removed at every depth, and data.layout removed
// because it is sent once in the layout frame instead. It is the one
// definition shared by keyframes and positional delta encoding, so delta
// positions always match the client's copy. The input is not modified.
func ClientView(doc map[string]interface{}) map[string]interface{} {
	view, _ := stripKey(doc, privateDataKey).(map[string]interface{})

	if data, ok := view["data"].(map[string]interface{}); ok {
		delete(data, "layout")
	}

	return view
}
