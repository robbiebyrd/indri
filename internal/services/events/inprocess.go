package events

import "context"

// InProcess is a single-instance Publisher: Publish delivers directly to the
// one in-memory Subscribe channel. Adequate when there is a single server
// process; multi-instance deployments need the Redis publisher so events reach
// connections held by other instances.
type InProcess struct {
	ch chan ChangeEvent
}

func NewInProcess() *InProcess {
	return &InProcess{ch: make(chan ChangeEvent, 256)}
}

func (p *InProcess) Publish(ctx context.Context, event ChangeEvent) error {
	select {
	case p.ch <- event:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *InProcess) Subscribe(_ context.Context) (<-chan ChangeEvent, error) {
	return p.ch, nil
}
