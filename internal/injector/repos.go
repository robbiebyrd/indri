package injector

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	postgresClient "github.com/robbiebyrd/indri/internal/clients/postgres"
	sqliteClient "github.com/robbiebyrd/indri/internal/clients/sqlite"
	envVars "github.com/robbiebyrd/indri/internal/repo/env"
	gameRepo "github.com/robbiebyrd/indri/internal/repo/game"
	scriptRepo "github.com/robbiebyrd/indri/internal/repo/script"
	sessionRepo "github.com/robbiebyrd/indri/internal/repo/session"
	userRepo "github.com/robbiebyrd/indri/internal/repo/user"
)

func GetRepos(
	ctx context.Context,
	clients *ClientsInjector,
	scriptFilePath string,
) (*ReposInjector, error) {
	if clients == nil {
		return nil, errors.New("clients were not passed to the repo injector")
	}

	env := envVars.GetEnv()

	var (
		gr    gameRepo.Storer
		ur    userRepo.Storer
		sr    sessionRepo.Storer
		sqlDB *sql.DB
		err   error
	)

	// TODO: Instead of calling the individual store constructors directly, consider using a factory pattern or a registry to dynamically select the appropriate store based on the environment variable.
	switch env.DBBackend {
	case "mongodb":
		// TODO: Instead of making three calls, the individual store constructors could be consolidated into a single factory function that returns all the necessary stores at once.
		gr, err = gameRepo.NewMongoStore(
			ctx,
			clients.MongoDBClient,
			clients.LockManager,
			clients.Publisher,
		)
		if err != nil {
			return nil, err
		}
		ur, err = userRepo.NewMongoStore(ctx, clients.MongoDBClient)
		if err != nil {
			return nil, err
		}
		sr, err = sessionRepo.NewMongoStore(ctx, clients.MongoDBClient)
		if err != nil {
			return nil, err
		}
	case "memory":
		gr, err = gameRepo.NewMemoryStore(ctx, clients.LockManager, clients.Publisher)
		if err != nil {
			return nil, err
		}
		ur, err = userRepo.NewMemoryStore(ctx)
		if err != nil {
			return nil, err
		}
		sr, err = sessionRepo.NewMemoryStore(ctx)
		if err != nil {
			return nil, err
		}
	case "sqlite":
		sqlDB, err = sqliteClient.Open(env.SQLitePath)
		if err != nil {
			return nil, fmt.Errorf("open sqlite %q: %w", env.SQLitePath, err)
		}
		gr, err = gameRepo.NewSQLiteStore(ctx, sqlDB, clients.LockManager, clients.Publisher)
		if err != nil {
			return nil, err
		}
		ur, err = userRepo.NewSQLiteStore(ctx, sqlDB)
		if err != nil {
			return nil, err
		}
		sr, err = sessionRepo.NewSQLiteStore(ctx, sqlDB)
		if err != nil {
			return nil, err
		}
	case "postgres":
		// The URI carries a password, so it is deliberately left out of the error.
		sqlDB, err = postgresClient.Open(ctx, env.PostgresURI)
		if err != nil {
			return nil, fmt.Errorf("open postgres: %w", err)
		}
		gr, err = gameRepo.NewPostgresStore(ctx, sqlDB, clients.LockManager, clients.Publisher)
		if err != nil {
			return nil, err
		}
		ur, err = userRepo.NewPostgresStore(ctx, sqlDB)
		if err != nil {
			return nil, err
		}
		sr, err = sessionRepo.NewPostgresStore(ctx, sqlDB)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unknown database backend %q", env.DBBackend)
	}

	scr, err := scriptRepo.NewStore(scriptFilePath)
	if err != nil {
		return nil, err
	}

	return &ReposInjector{
		EnvVars:     env,
		GameRepo:    gr,
		UserRepo:    ur,
		SessionRepo: sr,
		ScriptRepo:  scr,
		SQLDB:       sqlDB,
	}, nil
}
