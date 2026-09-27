package entrypoints

import (
	"log"

	"github.com/robbiebyrd/indri/internal/services/connection"
	gameService "github.com/robbiebyrd/indri/internal/services/game"
	sessionService "github.com/robbiebyrd/indri/internal/services/session"
	"github.com/robbiebyrd/indri/internal/transport"
)

func HandleConnect(
	s transport.Conn,
	t transport.Transport,
	gs *gameService.Service,
	ss *sessionService.Service,
) {
	cs := connection.NewService(s, t)

	err := cs.Write([]byte(`{ "stage": { "currentScene": "login"} }`))
	if err != nil {
		log.Printf("Error sending ready message to session: %v\n", err)
		HandleDisconnect(s, t, gs, ss)
	}
}

func HandleDisconnect(
	s transport.Conn,
	t transport.Transport,
	gs *gameService.Service,
	ss *sessionService.Service,
) {
	cs := connection.NewService(s, t)

	sessionId, err := cs.GetKeyAsString("sessionId")
	if err != nil || sessionId == nil {
		log.Printf("error getting standard session keys on disconnect: %v\n", err)
		return
	}

	// When the transport drives the disconnect it has already closed the
	// connection, so this only notifies and closes one we still own (logout).
	connection.Close(s)

	session, err := ss.Get(*sessionId)
	if err != nil {
		log.Printf("error getting session: %v\n", err)
		return
	} else if session.GameID == nil {
		log.Print("error getting gameId from session")
		return
	} else if session.SlotID == nil || *session.SlotID == "" {
		log.Print("session has no slotId; cannot mark player disconnected")
		return
	}

	err = gs.DisconnectPlayer(*session.GameID, *session.SlotID)
	if err != nil {
		log.Printf("could not set player as disconnected: %v\n", err)
	}
}
