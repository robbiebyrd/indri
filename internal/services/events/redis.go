package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/redis/go-redis/v9"
)

const (
	changesChannel    = "indri:changes"
	deliveriesChannel = "indri:deliveries"
)

// Redis is a multi-instance Bus backed by Redis Pub/Sub. Every instance
// subscribes to one channel; a message published on any instance is delivered
// to all instances, which then act on it locally.
type Redis[T any] struct {
	client  *redis.Client
	channel string
}

// NewRedis is the change-event bus.
func NewRedis(client *redis.Client) *Redis[ChangeEvent] {
	return &Redis[ChangeEvent]{client: client, channel: changesChannel}
}

// NewRedisDeliveries is the bus for messages addressed to sessions.
func NewRedisDeliveries(client *redis.Client) *Redis[Delivery] {
	return &Redis[Delivery]{client: client, channel: deliveriesChannel}
}

func (p *Redis[T]) Publish(ctx context.Context, message T) error {
	payload, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshaling %s message: %w", p.channel, err)
	}

	if err := p.client.Publish(ctx, p.channel, payload).Err(); err != nil {
		return fmt.Errorf("publishing to %s: %w", p.channel, err)
	}

	return nil
}

func (p *Redis[T]) Subscribe(ctx context.Context) (<-chan T, error) {
	sub := p.client.Subscribe(ctx, p.channel)

	// Wait for the subscription to be confirmed, so nothing published after
	// Subscribe returns can be missed.
	if _, err := sub.Receive(ctx); err != nil {
		_ = sub.Close()
		return nil, fmt.Errorf("subscribing to %s: %w", p.channel, err)
	}

	out := make(chan T, 256)

	go func() {
		defer close(out)
		defer func() { _ = sub.Close() }()

		for {
			msg, err := sub.ReceiveMessage(ctx)
			if err != nil {
				if ctx.Err() == nil {
					log.Printf("%s subscription error: %v", p.channel, err)
				}

				return
			}

			var message T
			if err := json.Unmarshal([]byte(msg.Payload), &message); err != nil {
				log.Printf("could not decode %s message: %v", p.channel, err)
				continue
			}

			select {
			case out <- message:
			case <-ctx.Done():
				return
			}
		}
	}()

	return out, nil
}
