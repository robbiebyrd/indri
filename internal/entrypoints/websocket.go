package entrypoints

import (
	"log"

	"github.com/olahol/melody"

	"github.com/robbiebyrd/indri/internal/services/connection"
	gameService "github.com/robbiebyrd/indri/internal/services/game"
	sessionService "github.com/robbiebyrd/indri/internal/services/session"
)

func HandleConnect(s *melody.Session, m *melody.Melody, gs *gameService.Service, ss *sessionService.Service) {
	cs := connection.NewService(s, m)

	err := cs.Write([]byte(`{ "stage": { "currentScene": "login"} }`))
	if err != nil {
		log.Printf("Error sending ready message to session: %v\n", err)
		HandleDisconnect(s, m, gs, ss)
	}
}

func HandleDisconnect(s *melody.Session, m *melody.Melody, gs *gameService.Service, ss *sessionService.Service) {
	cs := connection.NewService(s, m)

	sessionId, err := cs.GetKeyAsString("sessionId")
	if err != nil || sessionId == nil {
		log.Printf("error getting standard session keys on disconnect: %v\n", err)
		return
	}

	// When melody drives the disconnect it has already closed the session, so
	// only notify/close when we still own an open connection (e.g. a kick).
	if !s.IsClosed() {
		if err = cs.Write([]byte(`{"disconnected": true}`)); err != nil {
			log.Printf("error writing disconnected: %v\n", err)
		}

		if err = s.Close(); err != nil {
			log.Printf("error closing session: %v\n", err)
		}
	}

	session, err := ss.Get(*sessionId)
	if err != nil {
		log.Printf("error getting session: %v\n", err)
		return
	} else if session.GameID == nil {
		log.Print("error getting gameId from session")
		return
	} else if session.UserID == nil {
		log.Print("error getting gameId from session")
		return
	}

	err = gs.DisconnectPlayer(*session.GameID, *session.UserID)
	if err != nil {
		log.Printf("could not set player as disconnected: %v\n", err)
	}
}
