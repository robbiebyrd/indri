package leave

import (
	"fmt"
	"log"

	"github.com/olahol/melody"

	"github.com/robbiebyrd/indri/internal/injector"
	"github.com/robbiebyrd/indri/internal/services/connection"
)

type Handler struct {
	i *injector.Injector
}

func New(i *injector.Injector) *Handler {
	return &Handler{i}
}

func (h *Handler) Handle(
	s *melody.Session,
	_ map[string]interface{},
) error {
	cs := connection.NewService(s, h.i.MelodyClient)

	sessionId, err := cs.GetKeyAsString("sessionId")
	if err != nil {
		return err
	}

	session, err := h.i.SessionService.Get(*sessionId)
	if err != nil {
		return err
	}
	if session.GameID == nil || *session.GameID == "" {
		return fmt.Errorf("player is not in a game")
	}

	g, err := h.i.GameService.Get(*session.GameID)
	if err != nil {
		return err
	}

	err = h.i.GameService.RemovePlayer(g.ID.Hex(), *session.UserID)
	if err != nil {
		log.Printf("could not disconnect player %v from game %v: %v\n", *session.UserID, *session.GameID, err)
	}

	return nil
}
