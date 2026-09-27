package kick

import (
	"fmt"
	"log"

	"github.com/robbiebyrd/indri/internal/transport"

	"github.com/robbiebyrd/indri/internal/entrypoints"
	handlerUtils "github.com/robbiebyrd/indri/internal/handlers/utils"
	"github.com/robbiebyrd/indri/internal/injector"
	"github.com/robbiebyrd/indri/internal/services/connection"
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
	cs := connection.NewService(s, h.i.Transport)

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
	callerSessionId, err := cs.GetKeyAsString("sessionId")
	if err != nil {
		return fmt.Errorf("must be logged in to kick a player: %w", err)
	}

	callerSession, err := h.i.SessionService.Get(*callerSessionId)
	if err != nil {
		return fmt.Errorf("could not resolve calling session: %w", err)
	}

	if callerSession.SlotID == nil || *callerSession.SlotID == "" {
		return fmt.Errorf("calling session has no slot id")
	}

	g, err := h.i.GameService.GetByCode(*gameCode)
	if err != nil {
		return err
	}

	gameId := g.ID

	if callerSession.GameID == nil || *callerSession.GameID != gameId {
		return fmt.Errorf("caller is not in game %v", *gameCode)
	}

	callerSlot := *callerSession.SlotID
	if !g.Players[callerSlot].Host {
		return fmt.Errorf("caller %v is not the host of game %v", callerSlot, *gameCode)
	}

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

	// Force-disconnect the target if they are currently connected. A target
	// who is offline has still been removed from the game above.
	if userConnection, err := cs.Get(&targetSessionId); err == nil {
		entrypoints.HandleDisconnect(userConnection, h.i.Transport, h.i.GameService, h.i.SessionService)
	}

	return nil
}
