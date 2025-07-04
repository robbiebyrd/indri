package boot

import (
	"github.com/robbiebyrd/indri/internal/handlers/join"
	"github.com/robbiebyrd/indri/internal/handlers/login"
	"github.com/robbiebyrd/indri/internal/handlers/message"
	"github.com/robbiebyrd/indri/internal/handlers/register"
	"log"
)

func Register() error {
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
		log.Println("an error occurred while registering handlers")
		for _, err := range errs {
			log.Println(err)
		}

		panic("error during registering handlers, exiting")
	}

	return nil
}
