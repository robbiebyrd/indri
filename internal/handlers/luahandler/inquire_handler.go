package luahandler

import (
	"fmt"

	lua "github.com/yuin/gopher-lua"

	"github.com/robbiebyrd/indri/internal/transport"
)

// InquireNotificationHandler runs a Lua script as a fire-and-forget
// notification when an inquire action completes. It has no game context:
// only msg is available and game state cannot be mutated.
type InquireNotificationHandler struct {
	script string
}

func NewInquireNotificationHandler(script string) *InquireNotificationHandler {
	return &InquireNotificationHandler{script: script}
}

func (h *InquireNotificationHandler) Handle(s transport.Conn, decodedMsg map[string]interface{}) error {
	L := lua.NewState()
	defer L.Close()

	if err := L.DoString(stdlib); err != nil {
		return fmt.Errorf("loading stdlib: %w", err)
	}

	msgTable := L.NewTable()
	for k, v := range decodedMsg {
		msgTable.RawSetString(k, jsonToLua(L, v))
	}
	L.SetGlobal("msg", msgTable)

	if err := L.DoString(h.script); err != nil {
		return fmt.Errorf("running inquire notification script: %w", err)
	}

	return nil
}
