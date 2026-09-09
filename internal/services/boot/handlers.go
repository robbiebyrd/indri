package boot

import (
	"log"

	"github.com/robbiebyrd/indri/internal/entrypoints"
	"github.com/robbiebyrd/indri/internal/handlers/actions/create"
	"github.com/robbiebyrd/indri/internal/handlers/actions/inquire"
	"github.com/robbiebyrd/indri/internal/handlers/actions/join"
	"github.com/robbiebyrd/indri/internal/handlers/actions/kick"
	"github.com/robbiebyrd/indri/internal/handlers/actions/leave"
	"github.com/robbiebyrd/indri/internal/handlers/actions/login"
	"github.com/robbiebyrd/indri/internal/handlers/actions/logout"
	"github.com/robbiebyrd/indri/internal/handlers/actions/reconnect"
	"github.com/robbiebyrd/indri/internal/handlers/actions/refresh"
	"github.com/robbiebyrd/indri/internal/handlers/actions/register"
	"github.com/robbiebyrd/indri/internal/handlers/router"
	"github.com/robbiebyrd/indri/internal/injector"
	"github.com/robbiebyrd/indri/internal/transport"
)

func registerHandlers(i *injector.Injector) {
	i.Transport.Handle(transport.Handlers{
		Connect: func(c transport.Conn) {
			entrypoints.HandleConnect(c, i.Transport, i.GameService, i.SessionService)
		},
		Disconnect: func(c transport.Conn) {
			entrypoints.HandleDisconnect(c, i.Transport, i.GameService, i.SessionService)
		},
		Message: func(c transport.Conn, msg []byte) {
			router.HandleMessage(c, msg)
		},
		Error: func(c transport.Conn, err error) {
			log.Printf("client transport error: %v", err)
		},
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
			Name:    "indri_logout",
			Action:  "logout",
			Handler: logout.New(i),
		},
		{
			Name:    "indri_inquire",
			Action:  "inquire",
			Handler: inquire.New(i),
		},
	}

	router.RegisterHandlers(actionToHandlerMap)
}
