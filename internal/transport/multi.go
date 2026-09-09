package transport

import "net/http"

// Multi aggregates several transports behind one Transport, so the rest of the
// app (broadcast, boot) targets a single object while clients connect over
// WebSocket, GraphQL, etc. simultaneously. Lifecycle handlers and fan-out are
// delegated to every sub-transport; Conns are unioned.
type Multi struct {
	transports []Transport
}

func NewMulti(transports ...Transport) *Multi {
	return &Multi{transports: transports}
}

func (m *Multi) Handle(h Handlers) {
	for _, t := range m.transports {
		t.Handle(h)
	}
}

func (m *Multi) Register(mux *http.ServeMux) {
	for _, t := range m.transports {
		t.Register(mux)
	}
}

func (m *Multi) Broadcast(msg []byte) error {
	var firstErr error

	for _, t := range m.transports {
		if err := t.Broadcast(msg); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

func (m *Multi) BroadcastFilter(msg []byte, match func(Conn) bool) error {
	var firstErr error

	for _, t := range m.transports {
		if err := t.BroadcastFilter(msg, match); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

func (m *Multi) Conns() ([]Conn, error) {
	var all []Conn

	for _, t := range m.transports {
		conns, err := t.Conns()
		if err != nil {
			return nil, err
		}

		all = append(all, conns...)
	}

	return all, nil
}

func (m *Multi) Close() error {
	var firstErr error

	for _, t := range m.transports {
		if err := t.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

// IsClosed reports true only when every sub-transport is closed.
func (m *Multi) IsClosed() bool {
	for _, t := range m.transports {
		if !t.IsClosed() {
			return false
		}
	}

	return true
}
