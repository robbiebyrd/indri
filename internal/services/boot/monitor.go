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

			payload, err := broadcastPayload(event, i.GameService)
			if err != nil {
				log.Printf("could not build the broadcast for game %v: %v", gameID, err)
				continue
			}

			// Every instance receives each change, so each writes only to its
			// own connections.
			if err := i.BroadcastService.BroadcastLocal(&gameID, payload); err != nil {
				log.Printf("Error broadcasting change event: %v\n", err)
			}
		}
	}
}

// gamePayloads loads what a game's clients are sent when a change event asks
// for more than a delta.
type gamePayloads interface {
	Keyframe(id string) (events.KeyframeWrapper, error)
	LayoutFrame(id string) (events.LayoutFrame, error)
}

// broadcastPayload is what to send a game's clients for event: the delta
// itself; a fresh keyframe when a write changed the game's shape; or the
// game's layout frame when a write edited its layout.
func broadcastPayload(event events.ChangeEvent, load gamePayloads) (any, error) {
	switch event.OperationType {
	case events.OpKeyframe:
		return load.Keyframe(event.ID)
	case events.OpLayout:
		return load.LayoutFrame(event.ID)
	default:
		return event, nil
	}
}
