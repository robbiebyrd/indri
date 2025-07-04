package message

import (
	"fmt"
	"github.com/olahol/melody"
)

type RegisterHandlersInput struct {
	Action  string
	Handler actionHandlerFuncSig
}

type actionHandlerFuncSig func(
	s *melody.Session,
	decodedMsg map[string]interface{},
) error

var registeredHandlerMap = map[string]actionHandlerFuncSig{}

func RegisterHandler(action string, handler actionHandlerFuncSig) error {
	if _, ok := registeredHandlerMap[action]; ok {
		return fmt.Errorf("the action %v already exists in the handler map", action)
	}

	registeredHandlerMap[action] = handler

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

func Act(
	s *melody.Session,
	decodedMsg *map[string]interface{},
	action *string,
) error {
	if decodedMsg == nil {
		return fmt.Errorf("decoded message is nil")
	}

	var err error

	if _, ok := registeredHandlerMap[*action]; !ok {
		return fmt.Errorf("unknown action %v", *action)
	}

	err = registeredHandlerMap[*action](s, *decodedMsg)
	if err != nil {
		return err
	}

	return nil
}
