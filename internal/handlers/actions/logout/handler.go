package logout

import (
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
	// Not authenticated on this connection — nothing to invalidate.
	if req.Session == nil {
		return actions.Result{}, nil
	}

	session := req.Session

	// Mark the player disconnected while the session still exists, then
	// invalidate the session so its bearer token can't be replayed, then ask the
	// transport to close the connection.
	if session.GameID != nil && session.UserID != nil {
		if err := h.i.GameService.DisconnectPlayer(*session.GameID, *session.UserID); err != nil {
			log.Printf("logout: could not mark player disconnected: %v", err)
		}
	}

	if err := h.i.SessionService.Delete(session.ID); err != nil {
		log.Printf("logout: could not invalidate session %v: %v", session.ID, err)
	}

	return actions.Result{DisconnectIDs: []string{session.ID}}, nil
}
