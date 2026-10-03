package injector

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/redis/go-redis/v9"

	mongoClient "github.com/robbiebyrd/indri/internal/clients/mongodb"
	postgresClient "github.com/robbiebyrd/indri/internal/clients/postgres"
	redisClient "github.com/robbiebyrd/indri/internal/clients/redis"
	sqliteClient "github.com/robbiebyrd/indri/internal/clients/sqlite"
	envVars "github.com/robbiebyrd/indri/internal/repo/env"
	"github.com/robbiebyrd/indri/internal/services/events"
	"github.com/robbiebyrd/indri/internal/services/lock"
	"github.com/robbiebyrd/indri/internal/transport"
	"github.com/robbiebyrd/indri/internal/transport/ws"
)

var globalClientsInjector *ClientsInjector

func GetClients(ctx context.Context, mongodbClient *mongoClient.Client, clientTransport transport.Transport, lockManager lock.Manager, publisher events.Publisher) (*ClientsInjector, error) {
	if globalClientsInjector != nil {
		return globalClientsInjector, nil
	}

	var sqlDB *sql.DB

	if mongodbClient == nil {
		opened, db, err := openDatabase(ctx, envVars.GetEnv())
		if err != nil {
			return nil, err
		}

		mongodbClient, sqlDB = opened, db
	}

	if clientTransport == nil {
		clientTransport = ws.New()
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
			lockManager = lock.NewRedis(sharedRedis, lock.DefaultRedisLeaseTTL)
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
		SQLDB:         sqlDB,
		Transport:     clientTransport,
		LockManager:   lockManager,
		Publisher:     publisher,
	}, nil
}

// The database backends INDRI_DB_BACKEND names.
const (
	backendMongoDB  = "mongodb"
	backendMemory   = "memory"
	backendSQLite   = "sqlite"
	backendPostgres = "postgres"
)

// openDatabase connects to the one database the configured backend needs, and
// only that one: a SQLite server does not need MongoDB to be reachable, and the
// memory backend needs nothing at all.
func openDatabase(ctx context.Context, vars *envVars.Vars) (*mongoClient.Client, *sql.DB, error) {
	// Redis is the multi-instance switch. Storage held inside one process, or
	// in a file on one machine, cannot be shared between instances, so each
	// would keep its own users, sessions and games while sharing locks.
	if vars.LockBackend == "redis" && (vars.DBBackend == backendMemory || vars.DBBackend == backendSQLite) {
		return nil, nil, fmt.Errorf(
			"INDRI_LOCK_BACKEND is redis (multi-instance) but INDRI_DB_BACKEND is %s, whose data no other instance can see;"+
				" use mongodb or postgres", vars.DBBackend)
	}

	switch vars.DBBackend {
	case backendMongoDB:
		client, err := mongoClient.New(ctx)

		return client, nil, err
	case backendMemory:
		return nil, nil, nil
	case backendSQLite:
		db, err := sqliteClient.Open(vars.SQLitePath)
		if err != nil {
			return nil, nil, fmt.Errorf("opening the SQLite database at INDRI_SQLITE_PATH %q: %w", vars.SQLitePath, err)
		}

		return nil, db, nil
	case backendPostgres:
		if vars.PostgresURI == "" {
			return nil, nil, fmt.Errorf("INDRI_DB_BACKEND is postgres but INDRI_POSTGRES_URI is empty")
		}

		db, err := postgresClient.Open(ctx, vars.PostgresURI)
		if err != nil {
			return nil, nil, fmt.Errorf("connecting to PostgreSQL at INDRI_POSTGRES_URI: %w", err)
		}

		return nil, db, nil
	default:
		return nil, nil, fmt.Errorf(
			"INDRI_DB_BACKEND is %q; it must be one of %s, %s, %s or %s",
			vars.DBBackend, backendMongoDB, backendMemory, backendSQLite, backendPostgres)
	}
}
