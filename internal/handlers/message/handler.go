package message

import (
	"github.com/olahol/melody"
	"github.com/robbiebyrd/indri/internal/handlers/utils"
	"github.com/robbiebyrd/indri/internal/services/connection"
	gameService "github.com/robbiebyrd/indri/internal/services/game"
	"github.com/robbiebyrd/indri/internal/services/session"
	"log"
)

func HandleMessage(
	s *melody.Session,
	msg []byte,
) {
	gs := gameService.NewService()
	ss := session.NewService(s)
	cs := connection.NewService()

	// The first step in handling a message is to successfully decode its payload into a map[string]interface{}.
	// The map must have a string key named "action" that will be used to determine which handler should
	// process the message. The "action" key is deleted from the decoded message before it is passed to the handler;
	// the action will still be passed as a string, however, separate from the payload.
	action, decodedMsg, err := utils.DecodeMessageWithAction(msg)
	if err != nil {
		log.Printf("error decoding message %v: %v\n", string(msg), err)
		return
	}

	// TODO: In the future, we want to compare the and modified games to send a diff rather than the full scene payload.
	//originalgameCode, err := session.GetKeyAsString(s, "code")
	//if err != nil {
	//	log.Printf("error getting game id from session: %v\n", err)
	//}
	//
	// originalGame, err := gs.Get(originalgameCode)
	// if err != nil {
	// 	log.Printf("error getting originalGame with id %v: %v\n", originalgameCode, err)
	// }
	//

	// Next, we pass the message to Act, which decides which handler to invoke based on the incoming `action`
	// parameter in the message body.
	err = Act(s, decodedMsg, action)
	if err != nil {
		log.Printf("error handling message %v: %v\n", decodedMsg, err)
	}

	// After the message has been acted on, we need to refresh the current user's session keys, as they may have been
	// manipulated when the message was acted upon.
	gameCode, err := ss.GetKeyAsString("code")
	if err != nil {
		log.Printf("could not get gameCode from session: %v\n", err)
		return
	}

	// Get the modified game
	modifiedGame, err := gs.Fetch(gameCode)
	if err != nil {
		log.Printf("error getting game with id %v: %v\n", gameCode, err)
	}

	// Send a broadcast to all the game's players of the updated game.
	sanitizedGame := gs.Sanitize(modifiedGame)
	if err != nil {
		log.Printf("could not broadcast updated game %v: %v\n", sanitizedGame, err)
	}

	err = cs.Broadcast(gameCode, nil, sanitizedGame)
	if err != nil {
		log.Printf("error broadcasting message %v: %v\n", msg, err)
	}
}
