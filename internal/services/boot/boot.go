package boot

import (
	"context"
	"fmt"
	"github.com/robbiebyrd/indri/internal/entrypoints"
	cs "github.com/robbiebyrd/indri/internal/entrypoints/changestream"
	"github.com/robbiebyrd/indri/internal/entrypoints/http"
	"github.com/robbiebyrd/indri/internal/handlers/join"
	"github.com/robbiebyrd/indri/internal/handlers/login"
	"github.com/robbiebyrd/indri/internal/handlers/message"
	"github.com/robbiebyrd/indri/internal/handlers/register"
	"github.com/robbiebyrd/indri/internal/injector"
	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/services/connection"
	"github.com/robbiebyrd/indri/internal/services/game"
	"golang.org/x/sync/errgroup"
	"log"
)

func Start() {
	g, ctx := errgroup.WithContext(context.Background())

	g.Go(http.Serve)
	g.Go(func() error { return monitorGameChanges(ctx) })

	if err := g.Wait(); err != nil {
		fmt.Printf("One or more goroutines failed: %v\n", err)
	} else {
		fmt.Println("All goroutines completed successfully.")
	}

}
func Boot(script *models.Script) (*injector.Data, error) {
	i, err := injector.New(nil, nil, nil)
	if err != nil {
		log.Fatal(err)
	}

	registerHandlers(i, script)

	return i, nil

}

func registerHandlers(i *injector.Data, script *models.Script) {

	i.MelodyClient.HandleConnect(entrypoints.HandleConnect)
	i.MelodyClient.HandleDisconnect(entrypoints.HandleDisconnect)
	i.MelodyClient.HandleMessage(message.HandleMessage)

	actionToHandlerMap := []message.RegisterHandlersInput{
		{
			Action:  "register",
			Handler: register.HandleRegister,
		},
		{
			Action:  "join",
			Handler: join.HandleJoin,
		},
		{
			Action:  "leave",
			Handler: join.HandleLeave,
		},
		{
			Action:  "kick",
			Handler: join.HandleKick,
		},
		{
			Action:  "login",
			Handler: login.HandleLogin,
		},
	}

	errs := message.RegisterHandlers(actionToHandlerMap)
	if len(errs) > 0 {
		for _, err := range errs {
			log.Println(err)
		}

		panic("error during registering handlers, exiting")
	}

	_ = game.NewService(nil, script)
}

func monitorGameChanges(ctx context.Context) error {
	receiver := make(chan cs.ChangeEventOut)

	_, cancel := context.WithCancel(ctx)
	collection := "game"

	cm, err := cs.New(ctx, &collection, nil)
	if err != nil {
		cancel()
		return err
	}

	connService := connection.NewService()

	defer cancel()

	go cm.Monitor(ctx, receiver)

	for val := range receiver {
		err = connService.BroadcastToAll(val)
		if err != nil {
			fmt.Printf("Error broadcasting change event: %v\n", err)
		}

		fmt.Printf("Received change event: %v\n", val)
	}

	return nil
}
