package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/redis/go-redis/v9"
)

const redisChannel = "indri:changes"

// Redis is a multi-instance Publisher backed by Redis Pub/Sub. Every instance
// subscribes to one channel; a write on any instance is published once and
// delivered to all instances, which then fan out to their local websocket
// connections.
type Redis struct {
	client *redis.Client
}

func NewRedis(client *redis.Client) *Redis {
	return &Redis{client: client}
}

func (p *Redis) Publish(ctx context.Context, event ChangeEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshaling change event: %w", err)
	}

	if err := p.client.Publish(ctx, redisChannel, payload).Err(); err != nil {
		return fmt.Errorf("publishing change event: %w", err)
	}

	return nil
}

func (p *Redis) Subscribe(ctx context.Context) (<-chan ChangeEvent, error) {
	sub := p.client.Subscribe(ctx, redisChannel)

	out := make(chan ChangeEvent, 256)

	go func() {
		defer close(out)
		defer func() { _ = sub.Close() }()

		for {
			msg, err := sub.ReceiveMessage(ctx)
			if err != nil {
				if ctx.Err() == nil {
					log.Printf("change event subscription error: %v", err)
				}

				return
			}

			var event ChangeEvent
			if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil {
				log.Printf("could not decode change event: %v", err)
				continue
			}

			select {
			case out <- event:
			case <-ctx.Done():
				return
			}
		}
	}()

	return out, nil
}
