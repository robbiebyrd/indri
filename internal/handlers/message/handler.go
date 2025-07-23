package message

import (
	"github.com/olahol/melody"
	"github.com/robbiebyrd/indri/internal/handlers/utils"
	"log"
)

func HandleMessage(s *melody.Session, msg []byte) {
	// The first step in handling a message is to successfully decode its payload into a map[string]interface{}.
	// The map must have a string key named "action" that will be used to determine which handler should
	// process the message. The "action" key is deleted from the decoded message before it is passed to the handler;
	// the action will still be passed as a string, separate from the payload.
	action, decodedMsg, err := utils.DecodeMessageWithAction(msg)
	if err != nil {
		log.Printf("error decoding message %v: %v\n", string(msg), err)
		return
	}

	// Next, we pass the message to Act, which decides which handler to invoke based on the incoming `action`
	// parameter in the message body.
	err = Act(s, decodedMsg, action)
	if err != nil {
		log.Printf("error handling message %v: %v\n", decodedMsg, err)
	}
}
