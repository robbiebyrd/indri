package injector

import (
	"context"
	"time"

	"github.com/olahol/melody"

	mClient "github.com/robbiebyrd/indri/internal/clients/melody"
	mongoClient "github.com/robbiebyrd/indri/internal/clients/mongodb"
	redisClient "github.com/robbiebyrd/indri/internal/clients/redis"
	"github.com/robbiebyrd/indri/internal/entrypoints/changestream"
	envVars "github.com/robbiebyrd/indri/internal/repo/env"
	"github.com/robbiebyrd/indri/internal/services/lock"
)

var globalClientsInjector *ClientsInjector

func GetClients(ctx context.Context, mongodbClient *mongoClient.Client, melodyClient *melody.Melody, globalMonitor *changestream.MongoChangeMonitor, lockManager lock.Manager) (*ClientsInjector, error) {
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

	if globalMonitor == nil {
		newGlobalMonitor, err := changestream.New(ctx, mongodbClient, nil, nil)
		if err != nil {
			return nil, err
		}

		globalMonitor = newGlobalMonitor
	}

	if lockManager == nil {
		lm, err := newLockManager(ctx)
		if err != nil {
			return nil, err
		}

		lockManager = lm
	}

	return &ClientsInjector{
		MongoDBClient: mongodbClient,
		MelodyClient:  melodyClient,
		GlobalMonitor: globalMonitor,
		LockManager:   lockManager,
	}, nil
}

// newLockManager builds the mutation lock manager from configuration. The
// default in-process manager is correct for a single instance; multi-instance
// deployments set INDRI_LOCK_BACKEND=redis so edits are serialized across
// processes.
func newLockManager(ctx context.Context) (lock.Manager, error) {
	if envVars.GetEnv().LockBackend != "redis" {
		return lock.NewInProcess(), nil
	}

	client, err := redisClient.New(ctx)
	if err != nil {
		return nil, err
	}

	return lock.NewRedis(client, 10*time.Second), nil
}
