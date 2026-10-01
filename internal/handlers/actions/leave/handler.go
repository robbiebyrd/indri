package leave

import (
	"fmt"
	"log"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
	"github.com/robbiebyrd/indri/internal/injector"
)

type Handler struct {
	i *injector.Injector
}

func New(i *injector.Injector) *Handler {
	return &Handler{i}
}

func (h *Handler) Handle(req actions.Request) (actions.Result, error) {
	if req.Session == nil {
		return actions.Result{}, fmt.Errorf("not authenticated")
	}

	session := req.Session

	if session.GameID == nil || *session.GameID == "" {
		return actions.Result{}, fmt.Errorf("player is not in a game")
	}

	g, err := h.i.GameService.Get(*session.GameID)
	if err != nil {
		return actions.Result{}, err
	}

	if err = h.i.GameService.RemovePlayer(g.ID, *session.UserID); err != nil {
		log.Printf("could not disconnect player %v from game %v: %v\n", *session.UserID, *session.GameID, err)
	}

	return actions.Result{}, nil
}
