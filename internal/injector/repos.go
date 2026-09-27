package injector

import (
	"context"
	"errors"
	"fmt"

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
