package router

import (
	"fmt"
	"log"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
	"github.com/robbiebyrd/indri/internal/models"
)

// Dispatch runs the handlers registered for action (wrapped in the
// received/action/processed lifecycle), threading the session through and
// merging their Results. It is connection-independent: any transport resolves
// the session and supplies the decoded payload.
func Dispatch(session *models.Session, action string, payload map[string]interface{}) (actions.Result, error) {
	if action == "" {
		return actions.Result{}, fmt.Errorf("action is empty")
	}

	var merged actions.Result

	for _, phase := range []string{"received", action, "processed"} {
		for _, registered := range registeredHandlerMap {
			if registered.Action != phase {
				continue
			}

			res, err := invokeHandler(registered.Handler, actions.Request{Session: session, Payload: payload})

			merged.Responses = append(merged.Responses, res.Responses...)
			merged.DisconnectIDs = append(merged.DisconnectIDs, res.DisconnectIDs...)

			if res.Session != nil {
				// Auth was just established/refreshed; use it for the rest of
				// the lifecycle and report it back to the transport.
				merged.Session = res.Session
				session = res.Session
			}

			if err != nil {
				return merged, err
			}
		}
	}

	return merged, nil
}

// invokeHandler runs a single handler, converting any panic into an error so a
// malformed client message cannot crash the process.
func invokeHandler(h actions.MessageHandler, req actions.Request) (result actions.Result, err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("recovered from panic handling message: %v", r)
			err = fmt.Errorf("internal error handling message: %v", r)
		}
	}()

	return h.Handle(req)
}
