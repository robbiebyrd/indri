package injector

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	mongoClient "github.com/robbiebyrd/indri/internal/clients/mongodb"
	redisClient "github.com/robbiebyrd/indri/internal/clients/redis"
	envVars "github.com/robbiebyrd/indri/internal/repo/env"
	"github.com/robbiebyrd/indri/internal/services/events"
	"github.com/robbiebyrd/indri/internal/services/lock"
	"github.com/robbiebyrd/indri/internal/transport"
)

var globalClientsInjector *ClientsInjector

func GetClients(ctx context.Context, mongodbClient *mongoClient.Client, clientTransport transport.Transport, lockManager lock.Manager, publisher events.Publisher) (*ClientsInjector, error) {
	if globalClientsInjector != nil {
		return globalClientsInjector, nil
	}

	env := envVars.GetEnv()
	if mongodbClient == nil && env.DBBackend == "mongodb" {
		newMongodbClient, err := mongoClient.New(ctx)
		if err != nil {
			return nil, err
		}

		mongodbClient = newMongodbClient
	}

	if clientTransport == nil {
		built, err := newTransport(envVars.GetEnv())
		if err != nil {
			return nil, fmt.Errorf("building transports: %w", err)
		}

		clientTransport = built
	}

	// In redis (multi-instance) mode the lock manager, the change-event bus, and
	// the delivery relay share one Redis connection; otherwise the first two are
	// in-process and there is no relay.
	multiInstance := envVars.GetEnv().LockBackend == "redis"

	var sharedRedis *redis.Client

	if multiInstance {
		client, err := redisClient.New(ctx)
		if err != nil {
			return nil, err
		}

		sharedRedis = client
	}

	if lockManager == nil {
		if multiInstance {
			lockManager = lock.NewRedis(sharedRedis, 10*time.Second)
		} else {
			lockManager = lock.NewInProcess()
		}
	}

	if publisher == nil {
		if multiInstance {
			publisher = events.NewRedis(sharedRedis)
		} else {
			publisher = events.NewInProcess()
		}
	}

	var deliveries events.Bus[events.Delivery]
	if multiInstance {
		deliveries = events.NewRedisDeliveries(sharedRedis)
	}

	return &ClientsInjector{
		MongoDBClient: mongodbClient,
		Transport:     clientTransport,
		LockManager:   lockManager,
		Publisher:     publisher,
		Deliveries:    deliveries,
	}, nil
}
