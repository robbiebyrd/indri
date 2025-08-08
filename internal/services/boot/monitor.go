package boot

import (
	"context"
	"log"

	cs "github.com/robbiebyrd/indri/internal/entrypoints/changestream"
	"github.com/robbiebyrd/indri/internal/injector"
)

func monitorGameChanges(ctx context.Context, i *injector.Injector) error {
	receiver := make(chan cs.ChangeEventOut)

	_, cancel := context.WithCancel(ctx)
	defer cancel()

	go i.GlobalMonitor.Monitor(ctx, receiver)

	for val := range receiver {
		hexId := val.ID.Hex()

		err := i.BroadcastService.Broadcast(&hexId, nil, val)
		if err != nil {
			log.Printf("Error broadcasting change event: %v\n", err)
		}
	}

	return nil
}
