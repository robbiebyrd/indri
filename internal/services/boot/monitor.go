package boot

import (
	"context"
	"log"

	"github.com/robbiebyrd/indri/internal/injector"
)

// monitorGameChanges subscribes to the change-event bus and broadcasts each
// delta to the websocket clients of the affected game. Events are produced by
// the application at each write (see the game repo) and delivered here either
// in-process or, in multi-instance mode, via Redis Pub/Sub — no MongoDB change
// stream, and therefore no replica set required.
func monitorGameChanges(ctx context.Context, i *injector.Injector) error {
	events, err := i.Publisher.Subscribe(ctx)
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-events:
			if !ok {
				return nil
			}

			gameID := event.ID

			if err := i.BroadcastService.Broadcast(&gameID, nil, event); err != nil {
				log.Printf("Error broadcasting change event: %v\n", err)
			}
		}
	}
}
