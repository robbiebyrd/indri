package router

import (
	"fmt"
	"log"

	"github.com/robbiebyrd/indri/internal/transport"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
)

func Act(
	s transport.Conn,
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

func runHandler(s transport.Conn, decodedMsg *map[string]interface{}, action string) error {
	for _, i := range registeredHandlerMap {
		if i.Action == action {
			if err := invokeHandler(i.Handler, s, decodedMsg); err != nil {
				return err
			}
		}
	}

	return nil
}

// invokeHandler runs a single handler, converting any panic into an error so a
// malformed client message cannot crash the process or leak the connection
// (net/http's per-connection recover would otherwise skip the transport's cleanup).
func invokeHandler(h actions.MessageHandler, s transport.Conn, decodedMsg *map[string]interface{}) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("recovered from panic handling message: %v", r)
			err = fmt.Errorf("internal error handling message: %v", r)
		}
	}()

	return h.Handle(s, *decodedMsg)
}
