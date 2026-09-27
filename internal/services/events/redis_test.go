package events

import (
	"context"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// redisClient connects to INDRI_TEST_REDIS_URL (e.g. redis://localhost:6379/0),
// skipping the test when it is unset.
func redisClient(t *testing.T) *redis.Client {
	t.Helper()

	url := os.Getenv("INDRI_TEST_REDIS_URL")
	if url == "" {
		t.Skip("INDRI_TEST_REDIS_URL not set; skipping Redis integration test")
	}

	opts, err := redis.ParseURL(url)
	if err != nil {
		t.Fatalf("parsing INDRI_TEST_REDIS_URL: %v", err)
	}

	client := redis.NewClient(opts)
	t.Cleanup(func() { _ = client.Close() })

	return client
}

// receive subscribes two instances to bus, publishes message from one, and
// returns what each instance received.
func receive[T any](t *testing.T, newBus func() Bus[T], message T) []T {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var subs []<-chan T
	for range 2 {
		ch, err := newBus().Subscribe(ctx)
		if err != nil {
			t.Fatal(err)
		}
		subs = append(subs, ch)
	}

	if err := newBus().Publish(ctx, message); err != nil {
		t.Fatal(err)
	}

	var got []T
	for _, ch := range subs {
		select {
		case m := <-ch:
			got = append(got, m)
		case <-ctx.Done():
			t.Fatal("an instance never received the message")
		}
	}

	return got
}

func TestRedis_DeliveriesReachEveryInstance(t *testing.T) {
	client := redisClient(t)
	sent := Delivery{SessionIDs: []string{"s1", "s2"}, Payload: map[string]any{"board": []any{"X", ""}}}

	for _, got := range receive(t, func() Bus[Delivery] { return NewRedisDeliveries(client) }, sent) {
		if !reflect.DeepEqual(got, sent) {
			t.Fatalf("received %+v, want %+v", got, sent)
		}
	}
}

func TestRedis_ChangeEventsReachEveryInstance(t *testing.T) {
	client := redisClient(t)
	sent := ChangeEvent{ID: "g1", OperationType: OpUpdate, Timestamp: time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC),
		UpdatedFields: [][]interface{}{{"p", "x"}}}

	for _, got := range receive(t, func() Bus[ChangeEvent] { return NewRedis(client) }, sent) {
		if !reflect.DeepEqual(got, sent) {
			t.Fatalf("received %+v, want %+v", got, sent)
		}
	}
}
