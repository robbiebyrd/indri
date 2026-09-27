package injector

import (
	"context"
	"errors"
	"fmt"

	sqliteClient "github.com/robbiebyrd/indri/internal/clients/sqlite"
	envVars "github.com/robbiebyrd/indri/internal/repo/env"
	gameRepo "github.com/robbiebyrd/indri/internal/repo/game"
	scriptRepo "github.com/robbiebyrd/indri/internal/repo/script"
	sessionRepo "github.com/robbiebyrd/indri/internal/repo/session"
	userRepo "github.com/robbiebyrd/indri/internal/repo/user"
)

func GetRepos(ctx context.Context, clients *ClientsInjector, scriptFilePath string) (*ReposInjector, error) {
	if clients == nil {
		return nil, errors.New("clients were not passed to the repo injector")
	}

	env := envVars.GetEnv()

	var (
		gr  gameRepo.Storer
		ur  userRepo.Storer
		sr  sessionRepo.Storer
		err error
	)

	switch env.DBBackend {
	case "mongodb":
		gr, err = gameRepo.NewMongoStore(ctx, clients.MongoDBClient, clients.LockManager, clients.Publisher)
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
		sqliteDB, err := sqliteClient.Open(env.SQLitePath)
		if err != nil {
			return nil, fmt.Errorf("open sqlite %q: %w", env.SQLitePath, err)
		}
		gr, err = gameRepo.NewSQLiteStore(ctx, sqliteDB, clients.LockManager, clients.Publisher)
		if err != nil {
			return nil, err
		}
		ur, err = userRepo.NewSQLiteStore(ctx, sqliteDB)
		if err != nil {
			return nil, err
		}
		sr, err = sessionRepo.NewSQLiteStore(ctx, sqliteDB)
		if err != nil {
			return nil, err
		}
	case "postgres":
		gr, err = gameRepo.NewPostgresStore(ctx, env.PostgresURI, clients.LockManager, clients.Publisher)
		if err != nil {
			return nil, err
		}
		ur, err = userRepo.NewPostgresStore(ctx, env.PostgresURI)
		if err != nil {
			return nil, err
		}
		sr, err = sessionRepo.NewPostgresStore(ctx, env.PostgresURI)
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
	}, nil
}
