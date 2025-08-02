package logout

import (
	"github.com/olahol/melody"
	"github.com/robbiebyrd/indri/internal/entrypoints"
	"github.com/robbiebyrd/indri/internal/injector"
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
	entrypoints.HandleDisconnect(s, h.i.MelodyClient, h.i.GameService)
	return nil
}
