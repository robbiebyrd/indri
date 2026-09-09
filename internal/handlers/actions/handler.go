package actions

import (
	"github.com/robbiebyrd/indri/internal/transport"
)

type MessageHandler interface {
	Handle(s transport.Conn,
		decodedMsg map[string]interface{},
	) error
}
