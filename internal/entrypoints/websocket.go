package entrypoints

import (
	"github.com/olahol/melody"
	"github.com/robbiebyrd/indri/internal/services/session"
	"log"
)

func HandleConnect(s *melody.Session) {
	err := s.Write([]byte(`{ "ready": true }"`))
	if err != nil {
		log.Printf("Error sending ready message to session: %v\n", err)
	}
}

func HandleDisconnect(s *melody.Session) {
	//gs := gameService.NewService()
	//cs := connection.NewService()
	ss := session.NewService(s)

	gameCode, teamId, playerId, err := ss.GetStandardKeys()
	if err != nil || gameCode == nil || teamId == nil || playerId == nil {
		log.Printf("error getting standard session keys on disconnect: %v\n", err)
		return
	}

	err = s.Write([]byte(`{"disconnected": true}`))
	if err != nil {
		log.Printf("error writing disconnected: %v\n", err)
	}

	err = s.Close()
	if err != nil {
		log.Printf("error closing session: %v\n", err)
	}

	//g, err := gs.DisconnectPlayer(gameCode, teamId, playerId)
	//if err != nil {
	//	log.Printf("could not set player as disconnected: %v\n", err)
	//}
	//
	//if g != nil {
	//	err = gs.Update(gameCode, g)
	//	if err != nil {
	//		log.Printf("could not update game after player was disconnected: %v\n", err)
	//	}
	//}

	//err = cs.Broadcast(gameCode, nil, g)
	//if err != nil {
	//	log.Printf("error broadcoasting on disconnect of %v in game %v: %v\n", playerId, gameCode, err)
	//	return
	//}
}
