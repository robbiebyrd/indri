package entrypoints

import (
	"github.com/olahol/melody"
	gameService "github.com/robbiebyrd/indri/internal/services/game"
	"github.com/robbiebyrd/indri/internal/services/session"
	"log"
)

func HandleConnect(s *melody.Session) {
	err := s.Write([]byte(`{ "ready": true }"`))
	if err != nil {
		log.Printf("Error sending ready message to session: %v\n", err)
		HandleDisconnect(s)
	}
}

func HandleDisconnect(s *melody.Session) {
	DisconnectPlayer(s)
}

func DisconnectPlayer(s *melody.Session) {
	gs := gameService.NewService(nil, nil)
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

	g, err := gs.GetByCode(*gameCode)
	if err != nil {
		log.Printf("could not set get game: %v\n", err)
	}

	err = gs.DisconnectPlayer(g.ID.Hex(), *playerId)
	if err != nil {
		log.Printf("could not set player as disconnected: %v\n", err)
	}
}
