package injector

import (
	"context"
	"errors"

	envVars "github.com/robbiebyrd/indri/internal/repo/env"
	gameRepo "github.com/robbiebyrd/indri/internal/repo/game"
	scheduleRepo "github.com/robbiebyrd/indri/internal/repo/schedule"
	scriptRepo "github.com/robbiebyrd/indri/internal/repo/script"
	sessionRepo "github.com/robbiebyrd/indri/internal/repo/session"
	userRepo "github.com/robbiebyrd/indri/internal/repo/user"
)

func GetRepos(ctx context.Context, clients *ClientsInjector, scriptFilePath string) (*ReposInjector, error) {
	if clients == nil {
		return nil, errors.New("clients were not passed to the repo injector")
	}

	gr, err := gameRepo.NewStore(ctx, clients.MongoDBClient, clients.LockManager, clients.Publisher)
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

	// Mongo-backed rather than Redis-backed, so a script's timers work in the
	// default single-instance mode as well as under INDRI_LOCK_BACKEND=redis.
	sch, err := scheduleRepo.NewStore(ctx, clients.MongoDBClient, scheduleRepo.Config{})
	if err != nil {
		return nil, err
	}

	return &ReposInjector{
		EnvVars:      envVars.GetEnv(),
		GameRepo:     gr,
		UserRepo:     ur,
		SessionRepo:  sr,
		ScriptRepo:   scr,
		ScheduleRepo: sch,
	}, nil
}
