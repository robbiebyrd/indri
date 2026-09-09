package lock

import (
	"context"
	"sync"
)

// InProcess is a single-process Manager backed by keyed mutexes. It is correct
// only within one process, so it suits local development, tests, and
// single-instance deployments. Multi-instance deployments must use a
// distributed Manager (see Redis).
type InProcess struct {
	mu    sync.Mutex
	locks map[string]*refCounted
}

type refCounted struct {
	mu   sync.Mutex
	refs int
}

func NewInProcess() *InProcess {
	return &InProcess{locks: make(map[string]*refCounted)}
}

func (m *InProcess) Acquire(ctx context.Context, key string) (Handle, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	m.mu.Lock()
	entry, ok := m.locks[key]
	if !ok {
		entry = &refCounted{}
		m.locks[key] = entry
	}
	entry.refs++
	m.mu.Unlock()

	entry.mu.Lock()

	return &inProcessHandle{manager: m, key: key, entry: entry}, nil
}

// release drops a reference to the key's lock and removes the map entry once no
// one is waiting, so the map does not grow without bound.
func (m *InProcess) release(key string, entry *refCounted) {
	entry.mu.Unlock()

	m.mu.Lock()
	entry.refs--
	if entry.refs == 0 {
		delete(m.locks, key)
	}
	m.mu.Unlock()
}

type inProcessHandle struct {
	manager  *InProcess
	key      string
	entry    *refCounted
	released bool
}

func (h *inProcessHandle) Release(_ context.Context) error {
	if h.released {
		return nil
	}

	h.released = true
	h.manager.release(h.key, h.entry)

	return nil
}
