package actions

import (
	"github.com/olahol/melody"
)

type MessageHandler interface {
	Handle(s *melody.Session,
		decodedMsg map[string]interface{},
	) error
}
