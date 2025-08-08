package injector

import (
	"context"
	"errors"

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

	gr, err := gameRepo.NewStore(ctx, clients.MongoDBClient)
	if err != nil {
		return nil, err
	}

	ur, err := userRepo.NewStore(ctx, clients.MongoDBClient)
	if err != nil {
		return nil, err
	}

	sr, err := sessionRepo.NewStore(ctx, clients.MongoDBClient)
	if err != nil {
		return nil, err
	}

	scr, err := scriptRepo.NewStore(scriptFilePath)
	if err != nil {
		return nil, err
	}

	return &ReposInjector{
		EnvVars:     envVars.GetEnv(),
		GameRepo:    gr,
		UserRepo:    ur,
		SessionRepo: sr,
		ScriptRepo:  scr,
	}, nil
}
