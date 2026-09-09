package injector

import (
	"context"
	"time"

	"github.com/olahol/melody"
	"github.com/redis/go-redis/v9"

	mClient "github.com/robbiebyrd/indri/internal/clients/melody"
	mongoClient "github.com/robbiebyrd/indri/internal/clients/mongodb"
	redisClient "github.com/robbiebyrd/indri/internal/clients/redis"
	envVars "github.com/robbiebyrd/indri/internal/repo/env"
	"github.com/robbiebyrd/indri/internal/services/events"
	"github.com/robbiebyrd/indri/internal/services/lock"
)

var globalClientsInjector *ClientsInjector

func GetClients(ctx context.Context, mongodbClient *mongoClient.Client, melodyClient *melody.Melody, lockManager lock.Manager, publisher events.Publisher) (*ClientsInjector, error) {
	if globalClientsInjector != nil {
		return globalClientsInjector, nil
	}

	if mongodbClient == nil {
		newMongodbClient, err := mongoClient.New(ctx)
		if err != nil {
			return nil, err
		}

		mongodbClient = newMongodbClient
	}

	if melodyClient == nil {
		melodyClient = mClient.New()
	}

	// In redis (multi-instance) mode both the lock manager and the change-event
	// bus share one Redis connection; otherwise both are in-process.
	multiInstance := envVars.GetEnv().LockBackend == "redis"

	var sharedRedis *redis.Client

	if multiInstance && (lockManager == nil || publisher == nil) {
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

	return &ClientsInjector{
		MongoDBClient: mongodbClient,
		MelodyClient:  melodyClient,
		LockManager:   lockManager,
		Publisher:     publisher,
	}, nil
}
