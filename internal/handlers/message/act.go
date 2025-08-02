package message

import (
	"fmt"
	"github.com/olahol/melody"
)

func Act(
	s *melody.Session,
	decodedMsg *map[string]interface{},
	action *string,
) error {
	var err error

	if decodedMsg == nil {
		return fmt.Errorf("decoded message is nil")
	}

	if _, ok := registeredHandlerMap[*action]; !ok {
		return fmt.Errorf("unknown action %v", *action)
	}

	err = registeredHandlerMap[*action](s, *decodedMsg)
	if err != nil {
		return err
	}

	return nil
}
