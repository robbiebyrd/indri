package logout

import (
	"log"

	"github.com/olahol/melody"

	"github.com/robbiebyrd/indri/internal/entrypoints"
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
	// Capture the session id before disconnecting, so we can invalidate it
	// afterwards. HandleDisconnect needs the session to still exist (to mark
	// the player disconnected), so we delete it only after cleanup.
	cs := connection.NewService(s, h.i.MelodyClient)
	sessionId, err := cs.GetKeyAsString("sessionId")

	entrypoints.HandleDisconnect(s, h.i.MelodyClient, h.i.GameService, h.i.SessionService)

	if err != nil || sessionId == nil {
		// Never authenticated on this connection — nothing to invalidate.
		return nil
	}

	// Invalidate the session so its bearer token can no longer be replayed via
	// reconnect. This is the whole point of logout beyond a plain disconnect.
	if delErr := h.i.SessionService.Delete(*sessionId); delErr != nil {
		log.Printf("logout: could not invalidate session %v: %v", *sessionId, delErr)
	}

	return nil
}
