package lock

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// releaseScript deletes the key only if it still holds our token, so a handle
// whose lease already expired cannot delete a lock a different holder has since
// acquired.
var releaseScript = redis.NewScript(`
if redis.call("get", KEYS[1]) == ARGV[1] then
    return redis.call("del", KEYS[1])
end
return 0
`)

// Redis is a distributed Manager backed by Redis SET NX with a per-holder
// token. The lease TTL bounds how long a crashed holder can block others;
// callers must pair it with a version fence at the store so a lease that
// expires mid-mutation cannot cause a lost update.
type Redis struct {
	client *redis.Client
	ttl    time.Duration
	// retry is how long to wait between acquisition attempts while another
	// holder owns the lock.
	retry time.Duration
}

func NewRedis(client *redis.Client, ttl time.Duration) *Redis {
	return &Redis{client: client, ttl: ttl, retry: 25 * time.Millisecond}
}

func (m *Redis) Acquire(ctx context.Context, key string) (Handle, error) {
	token, err := newToken()
	if err != nil {
		return nil, err
	}

	redisKey := "lock:" + key

	for {
		ok, err := m.client.SetNX(ctx, redisKey, token, m.ttl).Result()
		if err != nil {
			return nil, fmt.Errorf("acquiring lock %q: %w", key, err)
		}

		if ok {
			return &redisHandle{client: m.client, key: redisKey, token: token}, nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(m.retry):
		}
	}
}

func newToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating lock token: %w", err)
	}

	return hex.EncodeToString(b), nil
}

type redisHandle struct {
	client   *redis.Client
	key      string
	token    string
	released bool
}

func (h *redisHandle) Release(ctx context.Context) error {
	if h.released {
		return nil
	}

	h.released = true

	if err := releaseScript.Run(ctx, h.client, []string{h.key}, h.token).Err(); err != nil && err != redis.Nil {
		return fmt.Errorf("releasing lock %q: %w", h.key, err)
	}

	return nil
}
