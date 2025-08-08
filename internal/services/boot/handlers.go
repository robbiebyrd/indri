package boot

import (
	"log"

	"github.com/olahol/melody"

	"github.com/robbiebyrd/indri/internal/entrypoints"
	"github.com/robbiebyrd/indri/internal/handlers/actions/create"
	"github.com/robbiebyrd/indri/internal/handlers/actions/inquire"
	"github.com/robbiebyrd/indri/internal/handlers/actions/join"
	"github.com/robbiebyrd/indri/internal/handlers/actions/kick"
	"github.com/robbiebyrd/indri/internal/handlers/actions/leave"
	"github.com/robbiebyrd/indri/internal/handlers/actions/login"
	"github.com/robbiebyrd/indri/internal/handlers/actions/refresh"
	"github.com/robbiebyrd/indri/internal/handlers/actions/register"
	"github.com/robbiebyrd/indri/internal/handlers/router"
	"github.com/robbiebyrd/indri/internal/injector"
)

func registerHandlers(i *injector.Injector) {
	i.MelodyClient.HandleConnect(func(s *melody.Session) {
		entrypoints.HandleConnect(s, i.MelodyClient, i.GameService, i.SessionService)
	})
	i.MelodyClient.HandleDisconnect(func(s *melody.Session) {
		entrypoints.HandleDisconnect(s, i.MelodyClient, i.GameService, i.SessionService)
	})
	i.MelodyClient.HandleMessage(func(s *melody.Session, msg []byte) {
		router.HandleMessage(s, msg)
	})

	actionToHandlerMap := []router.RegisterHandlersInput{
		{
			Action:  "refresh",
			Handler: refresh.New(i),
		},
		{
			Action:  "register",
			Handler: register.New(i),
		},
		{
			Action:  "create",
			Handler: create.New(i),
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
		{
			Action:  "inquire",
			Handler: inquire.New(i),
		},
	}

	errs := router.RegisterHandlers(actionToHandlerMap)
	if len(errs) > 0 {
		for _, err := range errs {
			log.Println(err)
		}

		panic("error during registering handlers, exiting")
	}
}
