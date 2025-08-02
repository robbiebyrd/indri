package entrypoints

import (
	"github.com/olahol/melody"
	gameService "github.com/robbiebyrd/indri/internal/services/game"
	"github.com/robbiebyrd/indri/internal/services/session"
	"log"
)

func HandleConnect(s *melody.Session, m *melody.Melody, gameService *gameService.Service) {
	err := s.Write([]byte(`{ "stage": { "currentScene": "login"} }`))
	if err != nil {
		log.Printf("Error sending ready message to session: %v\n", err)
		HandleDisconnect(s, m, gameService)
	}
}

func HandleDisconnect(s *melody.Session, m *melody.Melody, gameService *gameService.Service) {
	ss := session.NewService(s, m)

	gameId, teamId, playerId, err := ss.GetStandardKeys()
	if err != nil || gameId == nil || teamId == nil || playerId == nil {
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

	g, err := gameService.Fetch(*gameId)
	if err != nil {
		log.Printf("could not set get game: %v\n", err)
	}

	err = gameService.DisconnectPlayer(g.ID.Hex(), *playerId)
	if err != nil {
		log.Printf("could not set player as disconnected: %v\n", err)
	}
}
