package luahandler

import (
	"github.com/robbiebyrd/indri/internal/handlers/router"
	"github.com/robbiebyrd/indri/internal/injector"
)

// Register reads every handler defined in the script and wires it into the
// router so incoming WebSocket messages are dispatched to the right Lua script.
func Register(i *injector.Injector) {
	if i.Script == nil {
		return
	}
	for action, script := range i.Script.Handlers {
		router.RegisterHandler("script:"+action, action, New(i, script))
	}
}
