package boot

import (
	"context"
	"log"

	cs "github.com/robbiebyrd/indri/internal/entrypoints/changestream"
	"github.com/robbiebyrd/indri/internal/injector"
)

func monitorGameChanges(ctx context.Context, i *injector.Injector) error {
	receiver := make(chan cs.ChangeEventOut)

	// Monitor closes receiver when it returns (which it does once ctx is
	// cancelled), so both the ctx.Done and channel-closed cases end the loop.
	go i.GlobalMonitor.Monitor(ctx, receiver)

	for {
		select {
		case <-ctx.Done():
			return nil
		case val, ok := <-receiver:
			if !ok {
				return nil
			}

			hexId := val.ID.Hex()

			if err := i.BroadcastService.Broadcast(&hexId, nil, val); err != nil {
				log.Printf("Error broadcasting change event: %v\n", err)
			}
		}
	}
}
