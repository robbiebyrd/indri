package entrypoints

import (
	"log"

	"github.com/robbiebyrd/indri/internal/services/connection"
	gameService "github.com/robbiebyrd/indri/internal/services/game"
	sessionService "github.com/robbiebyrd/indri/internal/services/session"
	"github.com/robbiebyrd/indri/internal/transport"
)

func HandleConnect(s transport.Conn, t transport.Transport, gs *gameService.Service, ss *sessionService.Service) {
	cs := connection.NewService(s, t)

	err := cs.Write([]byte(`{ "stage": { "currentScene": "login"} }`))
	if err != nil {
		log.Printf("Error sending ready message to session: %v\n", err)
		HandleDisconnect(s, t, gs, ss)
	}
}

func HandleDisconnect(s transport.Conn, t transport.Transport, gs *gameService.Service, ss *sessionService.Service) {
	cs := connection.NewService(s, t)

	sessionId, err := cs.GetKeyAsString("sessionId")
	if err != nil || sessionId == nil {
		log.Printf("error getting standard session keys on disconnect: %v\n", err)
		return
	}

	// When the transport drives the disconnect it has already closed the
	// connection, so only notify/close when we still own an open one (e.g. a kick).
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
		log.Print("error getting userId from session")
		return
	} else if session.SlotID == nil {
		log.Print("error getting slotId from session")
		return
	}

	// One connection ending is not the player going offline while their
	// session is still connected over another: a newer WebSocket of theirs
	// replaced this one, or they failed over to another transport.
	if connectedElsewhere(s, t, *sessionId) {
		return
	}

	err = gs.DisconnectPlayer(*session.GameID, *session.SlotID, *session.UserID)
	if err != nil {
		log.Printf("could not set player as disconnected: %v\n", err)
	}
}

// connectedElsewhere reports whether a connection other than ended is open and
// bound to sessionId, on any of t's transports.
func connectedElsewhere(ended transport.Conn, t transport.Transport, sessionId string) bool {
	conns, err := t.Conns()
	if err != nil {
		return false
	}

	for _, c := range conns {
		if c == ended || c.IsClosed() {
			continue
		}

		if bound, ok := c.Get(transport.SessionIDKey); ok && bound == sessionId {
			return true
		}
	}

	return false
}
