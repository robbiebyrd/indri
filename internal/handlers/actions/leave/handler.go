package leave

import (
	"errors"
	"fmt"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
	"github.com/robbiebyrd/indri/internal/injector"
	gameRepo "github.com/robbiebyrd/indri/internal/repo/game"
)

type Handler struct {
	i *injector.Injector
}

func New(i *injector.Injector) *Handler {
	return &Handler{i}
}

// Handle frees the caller's seat and takes their session out of the game, so
// it stops receiving that game's broadcasts.
//
// A caller whose seat is already gone — they were kicked, or left from another
// connection, even a moment ago — has nothing to free and only leaves; a seat
// they still hold and that could not be freed is an error, because they are
// still in the game. Freeing is attempted rather than checked first, so a seat
// lost in between is still recognised as gone.
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

	if session.SlotID != nil && session.UserID != nil {
		err := h.i.GameService.RemovePlayer(g.ID, *session.SlotID, *session.UserID)
		if err != nil && !errors.Is(err, gameRepo.ErrSlotNotHeld) {
			return actions.Result{}, fmt.Errorf("freeing slot %v of game %v: %w", *session.SlotID, g.ID, err)
		}
	}

	if err := h.i.SessionService.LeaveGame(session.ID, g.ID); err != nil {
		return actions.Result{}, fmt.Errorf("taking session %v out of game %v: %w", session.ID, g.ID, err)
	}

	return actions.Result{}, nil
}
