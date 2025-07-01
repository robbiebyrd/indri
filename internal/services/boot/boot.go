package boot

import (
	"indri/internal/handlers/join"
	"indri/internal/handlers/login"
	"indri/internal/handlers/message"
	"log"
)

func Register() error {
	a := []message.RegisterHandlersInput{
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

	errs := message.RegisterHandlers(a)
	if len(errs) > 0 {
		for _, err := range errs {
			log.Println(err)
		}

		panic("error during loading handlers, exiting")
	}

	return nil
}
