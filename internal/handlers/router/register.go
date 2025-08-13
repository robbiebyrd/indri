package router

import (
	"github.com/robbiebyrd/indri/internal/handlers/actions"
)

type Handler struct {
	Name    string
	Action  string
	Handler actions.MessageHandler
}

var registeredHandlerMap []Handler

func RegisterHandler(name string, action string, handler actions.MessageHandler) {
	registeredHandlerMap = append(registeredHandlerMap, Handler{name, action, handler})
}

func RegisterHandlers(handlers []Handler) {
	for _, handler := range handlers {
		RegisterHandler(handler.Name, handler.Action, handler.Handler)
	}
}
