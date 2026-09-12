package injector

import (
	"context"
	"errors"

	authSevice "github.com/robbiebyrd/indri/internal/services/authentication"
	broadcastService "github.com/robbiebyrd/indri/internal/services/broadcast"
	gameService "github.com/robbiebyrd/indri/internal/services/game"
	sessionService "github.com/robbiebyrd/indri/internal/services/session"
	userService "github.com/robbiebyrd/indri/internal/services/user"
	"github.com/robbiebyrd/indri/internal/transport"
	graphqlTransport "github.com/robbiebyrd/indri/internal/transport/graphql"
	restTransport "github.com/robbiebyrd/indri/internal/transport/rest"
	sseTransport "github.com/robbiebyrd/indri/internal/transport/sse"
)

func GetServices(ctx context.Context, clients *ClientsInjector, repos *ReposInjector) (*ServicesInjector, error) {
	if clients == nil {
		return nil, errors.New("clients were not passed to the repo injector")
	}

	if repos == nil {
		return nil, errors.New("clients were not passed to the repo injector")
	}

	// These transports need the session store (to authenticate bearer tokens),
	// which only exists now, so they are built here and aggregated with the
	// WebSocket transport from GetClients. Everything downstream targets the
	// aggregate.
	//
	// sse is push-only and rest is request-only: together they are the third
	// way to play, alongside WebSocket and GraphQL.
	gql := graphqlTransport.New(repos.SessionRepo)
	events := sseTransport.New(repos.SessionRepo)
	api := restTransport.New(repos.SessionRepo)

	multi := transport.NewMulti(clients.Transport, gql, events, api)

	// Each needs the aggregate so a kick closes the target's connections on
	// every transport, not just its own.
	for _, t := range []interface{ SetPeer(transport.Transport) }{gql, events, api} {
		t.SetPeer(multi)
	}

	clients.Transport = multi

	gs, err := gameService.NewService(repos.GameRepo, repos.ScriptRepo)
	if err != nil {
		return nil, err
	}

	bs, err := broadcastService.NewService(ctx, clients.Transport, repos.UserRepo, repos.SessionRepo)
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
