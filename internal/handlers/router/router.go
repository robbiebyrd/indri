package router

import (
	"github.com/robbiebyrd/indri/internal/handlers/actions"
	"github.com/robbiebyrd/indri/internal/handlers/utils"
	"github.com/robbiebyrd/indri/internal/models"
)

// DispatchMessage decodes a raw client message (which carries its own "action"
// field) and dispatches it for the given session. It is the entry point for
// message-oriented transports (WebSocket); request/response transports that
// already know the action (GraphQL mutations) call Dispatch directly.
func DispatchMessage(session *models.Session, msg []byte) (actions.Result, error) {
	action, decodedMsg, err := utils.DecodeMessageWithAction(msg)
	if err != nil {
		return actions.Result{}, err
	}

	return Dispatch(session, *action, *decodedMsg)
}
