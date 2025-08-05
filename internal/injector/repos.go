package injector

import (
	"context"
	"errors"
	envVars "github.com/robbiebyrd/indri/internal/repo/env"
	gameRepo "github.com/robbiebyrd/indri/internal/repo/game"
	userRepo "github.com/robbiebyrd/indri/internal/repo/user"
)

func GetRepos(ctx context.Context, clients *ClientsInjector) (*ReposInjector, error) {
	if clients == nil {
		return nil, errors.New("clients were not passed to the repo injector")
	}

	gr, err := gameRepo.NewRepo(ctx, clients.MongoDBClient)
	if err != nil {
		return nil, err
	}

	ur := userRepo.NewRepo(ctx, clients.MongoDBClient)

	return &ReposInjector{
		EnvVars:  envVars.GetEnv(),
		GameRepo: gr,
		UserRepo: ur,
	}, nil
}
