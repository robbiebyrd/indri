package injector

import (
	"context"
	"errors"

	authSevice "github.com/robbiebyrd/indri/internal/services/authentication"
	broadcastService "github.com/robbiebyrd/indri/internal/services/broadcast"
	gameService "github.com/robbiebyrd/indri/internal/services/game"
	sessionService "github.com/robbiebyrd/indri/internal/services/session"
	userService "github.com/robbiebyrd/indri/internal/services/user"
)

func GetServices(ctx context.Context, clients *ClientsInjector, repos *ReposInjector) (*ServicesInjector, error) {
	if clients == nil {
		return nil, errors.New("clients were not passed to the repo injector")
	}

	if repos == nil {
		return nil, errors.New("clients were not passed to the repo injector")
	}

	gs, err := gameService.NewService(repos.GameRepo, repos.ScriptRepo)
	if err != nil {
		return nil, err
	}

	bs, err := broadcastService.NewService(ctx, clients.MelodyClient, repos.UserRepo, repos.SessionRepo)
	if err != nil {
		return nil, err
	}

	as, err := authSevice.NewService(repos.UserRepo, repos.SessionRepo)
	if err != nil {
		return nil, err
	}

	us, err := userService.NewService(repos.UserRepo)
	if err != nil {
		return nil, err
	}

	ss := sessionService.NewService(repos.SessionRepo)

	return &ServicesInjector{
		GameService:      gs,
		BroadcastService: bs,
		AuthService:      as,
		UserService:      us,
		SessionService:   ss,
	}, nil
}
