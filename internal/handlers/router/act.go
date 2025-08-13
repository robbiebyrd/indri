package router

import (
	"fmt"

	"github.com/olahol/melody"
)

func Act(
	s *melody.Session,
	decodedMsg *map[string]interface{},
	action *string,
) error {
	if decodedMsg == nil {
		return fmt.Errorf("decoded message is nil")
	}

	if action == nil || *action == "" {
		return fmt.Errorf("action is nil or empty string")
	}

	actions := []string{"received", *action, "processed"}

	for _, a := range actions {
		err := runHandler(s, decodedMsg, a)
		if err != nil {
			return err
		}
	}

	return nil
}

func runHandler(s *melody.Session, decodedMsg *map[string]interface{}, action string) error {
	for _, i := range registeredHandlerMap {
		if i.Action == action {
			err := i.Handler.Handle(s, *decodedMsg)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
