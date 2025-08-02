package injector

import (
	"context"
	"errors"
	"github.com/robbiebyrd/indri/internal/repo/env"
	"github.com/robbiebyrd/indri/internal/repo/game"
	"github.com/robbiebyrd/indri/internal/repo/user"
)

func GetRepos(ctx context.Context, clients *ClientsInjector) (*ReposInjector, error) {
	if clients == nil {
		return nil, errors.New("clients were not passed to the repo injector")
	}

	gameRepo, err := game.NewRepo(ctx, clients.MongoDBClient)
	if err != nil {
		return nil, err
	}

	userRepo := user.NewRepo(ctx, clients.MongoDBClient)

	return &ReposInjector{
		EnvVars:  env.GetEnv(),
		GameRepo: gameRepo,
		UserRepo: userRepo,
	}, nil
}
