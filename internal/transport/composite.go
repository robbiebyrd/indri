package transport

import (
	"errors"
	"net/http"
)

type composite []Transport

// Composite runs several transports as one, so clients can connect over any
// of them concurrently while everything above this package keeps seeing a
// single Transport. Each child must mount distinct routes.
//
// Fan-out is best effort: one child failing never stops the others, and the
// failures are joined into the returned error.
func Composite(ts ...Transport) Transport {
	return composite(ts)
}

func (c composite) Handle(h Handlers) {
	for _, t := range c {
		t.Handle(h)
	}
}

func (c composite) Register(mux *http.ServeMux) {
	for _, t := range c {
		t.Register(mux)
	}
}

func (c composite) Broadcast(msg []byte) error {
	var errs []error
	for _, t := range c {
		errs = append(errs, t.Broadcast(msg))
	}

	return errors.Join(errs...)
}

func (c composite) BroadcastFilter(msg []byte, match func(Conn) bool) error {
	var errs []error
	for _, t := range c {
		errs = append(errs, t.BroadcastFilter(msg, match))
	}

	return errors.Join(errs...)
}

// Conns returns the connections of every child that could list them, plus
// the joined errors of those that couldn't.
func (c composite) Conns() ([]Conn, error) {
	var (
		all  []Conn
		errs []error
	)

	for _, t := range c {
		conns, err := t.Conns()
		all = append(all, conns...)
		errs = append(errs, err)
	}

	return all, errors.Join(errs...)
}

// Close closes every child that is still open. Shutdown may call it more than
// once, and re-closing a closed child is an error for some implementations.
func (c composite) Close() error {
	var errs []error
	for _, t := range c {
		if !t.IsClosed() {
			errs = append(errs, t.Close())
		}
	}

	return errors.Join(errs...)
}

func (c composite) IsClosed() bool {
	for _, t := range c {
		if !t.IsClosed() {
			return false
		}
	}

	return true
}
