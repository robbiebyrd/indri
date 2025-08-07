package injector

import (
	"context"
	"errors"

	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/services/broadcast"
	gameService "github.com/robbiebyrd/indri/internal/services/game"
	userService "github.com/robbiebyrd/indri/internal/services/user"
)

func GetServices(ctx context.Context, gameScript *models.Script, clients *ClientsInjector, repos *ReposInjector) (*ServicesInjector, error) {
	if clients == nil {
		return nil, errors.New("clients were not passed to the repo injector")
	}

	if repos == nil {
		return nil, errors.New("clients were not passed to the repo injector")
	}

	if gameScript == nil {
		return nil, errors.New("game script was not passed to the repo injector")
	}

	gs, err := gameService.NewService(repos.GameRepo, gameScript)
	if err != nil {
		return nil, err
	}

	cs, err := broadcast.NewService(ctx, clients.MelodyClient, repos.UserRepo)
	if err != nil {
		return nil, err
	}

	return &ServicesInjector{
		GameService:       gs,
		ConnectionService: cs,
		UserService:       userService.NewService(repos.UserRepo),
	}, nil
}
