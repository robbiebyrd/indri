package router

import (
	"fmt"
	"github.com/olahol/melody"
	"github.com/robbiebyrd/indri/internal/handlers/actions"
)

type RegisterHandlersInput struct {
	Action  string
	Handler actions.MessageHandler
}

type actionHandlerFuncSig func(
	s *melody.Session,
	decodedMsg map[string]interface{},
) error

var registeredHandlerMap = map[string]actionHandlerFuncSig{}

func RegisterHandler(action string, handler actions.MessageHandler) error {
	if _, ok := registeredHandlerMap[action]; ok {
		return fmt.Errorf("the action %v already exists in the handler map", action)
	}

	registeredHandlerMap[action] = handler.Handle

	return nil

}

func RegisterHandlers(handlers []RegisterHandlersInput) []error {
	var errs []error

	for _, handler := range handlers {
		err := RegisterHandler(handler.Action, handler.Handler)
		if err != nil {
			errs = append(errs, err)
		}
	}

	return errs
}
