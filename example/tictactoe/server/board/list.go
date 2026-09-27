// Package board holds helpers shared by the tic-tac-toe handlers.
package board

import "reflect"

// List returns v as a []any whatever slice type the game store decoded it
// as: MongoDB yields bson.A, the other stores []any. It reports false for
// anything that isn't a slice.
func List(v any) ([]any, bool) {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice {
		return nil, false
	}

	out := make([]any, rv.Len())
	for i := range out {
		out[i] = rv.Index(i).Interface()
	}

	return out, true
}
