package boot

import (
	"context"
	"log"
	"os"

	"github.com/olahol/melody"
	"golang.org/x/sync/errgroup"

	"github.com/robbiebyrd/indri/internal/entrypoints"
	cs "github.com/robbiebyrd/indri/internal/entrypoints/changestream"
	"github.com/robbiebyrd/indri/internal/entrypoints/http"
	"github.com/robbiebyrd/indri/internal/handlers/actions/join"
	"github.com/robbiebyrd/indri/internal/handlers/actions/kick"
	"github.com/robbiebyrd/indri/internal/handlers/actions/leave"
	"github.com/robbiebyrd/indri/internal/handlers/actions/login"
	"github.com/robbiebyrd/indri/internal/handlers/actions/refresh"
	"github.com/robbiebyrd/indri/internal/handlers/actions/register"
	"github.com/robbiebyrd/indri/internal/handlers/message"
	"github.com/robbiebyrd/indri/internal/injector"
	scriptRepo "github.com/robbiebyrd/indri/internal/repo/script"
	"github.com/robbiebyrd/indri/internal/services/connection"
)

func Start(i *injector.Injector) {
	g, ctx := errgroup.WithContext(i.GlobalContext)

	g.Go(func() error { return http.Serve(i.MelodyClient) })
	g.Go(func() error {
		return monitorGameChanges(ctx, i.ConnectionService, i.GlobalMonitor)
	})

	if err := g.Wait(); err != nil {
		log.Printf("One or more goroutines failed: %v\n", err)
	} else {
		log.Println("All goroutines completed successfully.")
	}
}

func Boot(scriptFilePath *string) (*injector.Injector, error) {
	ctx := context.Background()

	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	if scriptFilePath == nil || *scriptFilePath == "" {
		s := dir + "/config.json"
		scriptFilePath = &s
	}

	gameScript := scriptRepo.Get(*scriptFilePath)

	clients, err := injector.GetClients(ctx, nil, nil, nil)
	if err != nil {
		log.Fatal(err)
	}

	repos, err := injector.GetRepos(ctx, clients)
	if err != nil {
		log.Fatal(err)
	}

	services, err := injector.GetServices(ctx, gameScript, clients, repos)
	if err != nil {
		log.Fatal(err)
	}

	i := &injector.Injector{
		ReposInjector:    repos,
		ClientsInjector:  clients,
		ServicesInjector: services,
		GlobalContext:    ctx,
	}

	registerHandlers(i)

	return i, nil
}

func registerHandlers(i *injector.Injector) {

	i.MelodyClient.HandleConnect(func(s *melody.Session) {
		entrypoints.HandleConnect(s, i.MelodyClient, i.GameService)
	})
	i.MelodyClient.HandleDisconnect(func(s *melody.Session) {
		entrypoints.HandleDisconnect(s, i.MelodyClient, i.GameService)
	})
	i.MelodyClient.HandleMessage(func(s *melody.Session, msg []byte) {
		message.HandleMessage(s, msg)
	})

	actionToHandlerMap := []message.RegisterHandlersInput{
		{
			Action:  "refresh",
			Handler: refresh.New(i),
		},
		{
			Action:  "register",
			Handler: register.New(i),
		},
		{
			Action:  "join",
			Handler: join.New(i),
		},
		{
			Action:  "leave",
			Handler: leave.New(i),
		},
		{
			Action:  "kick",
			Handler: kick.New(i),
		},
		{
			Action:  "login",
			Handler: login.New(i),
		},
	}

	errs := message.RegisterHandlers(actionToHandlerMap)
	if len(errs) > 0 {
		for _, err := range errs {
			log.Println(err)
		}

		panic("error during registering handlers, exiting")
	}
}

func monitorGameChanges(ctx context.Context, connService *connection.Service, changeMonitor *cs.MongoChangeMonitor) error {
	receiver := make(chan cs.ChangeEventOut)

	_, cancel := context.WithCancel(ctx)
	defer cancel()

	go changeMonitor.Monitor(ctx, receiver)

	for val := range receiver {
		hexId := val.ID.Hex()

		err := connService.Broadcast(&hexId, nil, val)
		if err != nil {
			log.Printf("Error broadcasting change event: %v\n", err)
		}
	}

	return nil
}
