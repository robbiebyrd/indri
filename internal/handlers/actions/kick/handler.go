package kick

import (
	"fmt"
	"log"

	"github.com/robbiebyrd/indri/internal/transport"

	handlerUtils "github.com/robbiebyrd/indri/internal/handlers/utils"
	"github.com/robbiebyrd/indri/internal/injector"
)

type Handler struct {
	i *injector.Injector
}

func New(i *injector.Injector) *Handler {
	return &Handler{i}
}

// Handle processes a kick request and removes a player from a game if the requesting player is host.
func (h *Handler) Handle(
	s transport.Conn,
	decodedMsg map[string]interface{},
) error {
	gameCode, err := handlerUtils.RequireGameCode(decodedMsg)
	if err != nil {
		return err
	}

	targetSlotId, ok := decodedMsg["slotId"].(string)
	if !ok || targetSlotId == "" {
		return fmt.Errorf("slotId to kick must be provided")
	}

	// Authorize the CALLER from their own connection, never from the
	// client-supplied target slotId.
	g, _, err := handlerUtils.RequireHost(h.i, s, *gameCode)
	if err != nil {
		return err
	}

	gameId := g.ID

	// Resolve the target slot and find their session.
	targetPlayer, ok := g.Players[targetSlotId]
	if !ok || targetPlayer.UserID == "" {
		return fmt.Errorf("slot %v is empty or not found in game %v", targetSlotId, *gameCode)
	}

	targetSession, err := h.i.SessionService.GetByUserID(targetPlayer.UserID)
	if err != nil {
		return err
	}

	if targetSession.GameID == nil || *targetSession.GameID != gameId {
		return fmt.Errorf("target %v is not in game %v", targetSlotId, *gameCode)
	}

	targetSessionId := targetSession.ID

	if err = h.i.GameRepo.RemovePlayer(gameId, targetSlotId); err != nil {
		log.Printf("could not remove player %v from game %v: %v\n", targetSlotId, gameId, err)
	}

	// Disconnect the target on whichever instance holds their connection. A
	// target who is offline has still been removed from the game above.
	if err = h.i.BroadcastService.CloseSessions(targetSessionId); err != nil {
		log.Printf("could not disconnect kicked session %v: %v\n", targetSessionId, err)
	}

	return nil
}
