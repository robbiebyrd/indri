package boot

import (
	"github.com/olahol/melody"

	"github.com/robbiebyrd/indri/internal/entrypoints"
	"github.com/robbiebyrd/indri/internal/handlers/actions/create"
	"github.com/robbiebyrd/indri/internal/handlers/actions/inquire"
	"github.com/robbiebyrd/indri/internal/handlers/actions/join"
	"github.com/robbiebyrd/indri/internal/handlers/actions/kick"
	"github.com/robbiebyrd/indri/internal/handlers/actions/leave"
	"github.com/robbiebyrd/indri/internal/handlers/actions/login"
	"github.com/robbiebyrd/indri/internal/handlers/actions/reconnect"
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

	actionToHandlerMap := []router.Handler{
		{
			Name:    "indri_refresh",
			Action:  "refresh",
			Handler: refresh.New(i),
		},
		{
			Name:    "indri_register",
			Action:  "register",
			Handler: register.New(i),
		},
		{
			Name:    "indri_create",
			Action:  "create",
			Handler: create.New(i),
		},
		{
			Name:    "indri_join",
			Action:  "join",
			Handler: join.New(i),
		},
		{
			Name:    "indri_reconnect",
			Action:  "reconnect",
			Handler: reconnect.New(i),
		},
		{
			Name:    "indri_leave",
			Action:  "leave",
			Handler: leave.New(i),
		},
		{
			Name:    "indri_kick",
			Action:  "kick",
			Handler: kick.New(i),
		},
		{
			Name:    "indri_login",
			Action:  "login",
			Handler: login.New(i),
		},
		{
			Name:    "indri_inquire",
			Action:  "inquire",
			Handler: inquire.New(i),
		},
	}

	router.RegisterHandlers(actionToHandlerMap)
}
