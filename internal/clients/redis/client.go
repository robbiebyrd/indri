package redis

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"

	envVars "github.com/robbiebyrd/indri/internal/repo/env"
)

// New connects to Redis using the INDRI_REDIS_* environment settings and
// verifies the connection with a ping.
func New(ctx context.Context) (*redis.Client, error) {
	vars := envVars.GetEnv()

	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", vars.RedisHost, vars.RedisPort),
		Password: vars.RedisPassword,
		DB:       vars.RedisDatabase,
	})

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("could not connect to Redis: %w", err)
	}

	return client, nil
}
