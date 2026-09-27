package boot

import (
	"context"
	"log"

	"github.com/robbiebyrd/indri/internal/injector"
	"github.com/robbiebyrd/indri/internal/services/events"
)

// monitorGameChanges subscribes to the change-event bus and broadcasts each
// delta to the websocket clients of the affected game. Events are produced by
// the application at each write (see the game repo) and delivered here either
// in-process or, in multi-instance mode, via Redis Pub/Sub — no MongoDB change
// stream, and therefore no replica set required.
func monitorGameChanges(ctx context.Context, i *injector.Injector) error {
	changes, err := i.Publisher.Subscribe(ctx)
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-changes:
			if !ok {
				return nil
			}

			gameID := event.ID

			payload, err := broadcastPayload(event, func(id string) (events.KeyframeWrapper, error) {
				return i.GameService.Keyframe(id, i.LayoutHash)
			})
			if err != nil {
				log.Printf("could not build keyframe for game %v: %v", gameID, err)
				continue
			}

			if err := i.BroadcastService.Broadcast(&gameID, nil, payload); err != nil {
				log.Printf("Error broadcasting change event: %v\n", err)
			}
		}
	}
}

// broadcastPayload is what to send a game's clients for event: the delta
// itself, or, when a write changed the game's shape, a fresh keyframe loaded
// from the saved game.
func broadcastPayload(event events.ChangeEvent, keyframe func(id string) (events.KeyframeWrapper, error)) (any, error) {
	if event.OperationType != events.OpKeyframe {
		return event, nil
	}

	return keyframe(event.ID)
}
