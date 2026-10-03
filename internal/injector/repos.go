package injector

import (
	"context"
	"errors"

	envVars "github.com/robbiebyrd/indri/internal/repo/env"
	scriptRepo "github.com/robbiebyrd/indri/internal/repo/script"
)

func GetRepos(ctx context.Context, clients *ClientsInjector, scriptFilePath string) (*ReposInjector, error) {
	if clients == nil {
		return nil, errors.New("clients were not passed to the repo injector")
	}

	env := envVars.GetEnv()

	stores, err := openStores(ctx, env.DBBackend, clients)
	if err != nil {
		return nil, err
	}

	scr, err := scriptRepo.NewStore(scriptFilePath)
	if err != nil {
		return nil, err
	}

	return &ReposInjector{
		EnvVars:      env,
		GameRepo:     stores.game,
		UserRepo:     stores.user,
		SessionRepo:  stores.session,
		ScriptRepo:   scr,
		ScheduleRepo: stores.schedule,
	}, nil
}
