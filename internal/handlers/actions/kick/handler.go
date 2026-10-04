package kick

import (
	"fmt"
	"log"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
	handlerUtils "github.com/robbiebyrd/indri/internal/handlers/utils"
	"github.com/robbiebyrd/indri/internal/injector"
	"github.com/robbiebyrd/indri/internal/models"
)

// games is the narrow view of the game service a kick needs: find the game the
// caller named, and free a slot in it while the caller is still its host.
type games interface {
	GetByCode(gameCode string) (*models.Game, error)
	RemovePlayerIf(id string, slotId string, userId string, allowed func(*models.Game) error) error
}

// sessions is the narrow view of the session service a kick needs: resolve the
// target's session, which carries both the game the target is in and the id the
// transport disconnects, and take it out of the game.
type sessions interface {
	GetByUserID(userId string) (*models.Session, error)
	LeaveGame(sessionId string, gameId string) error
}

type Handler struct {
	i *injector.Injector
}

func New(i *injector.Injector) *Handler {
	return &Handler{i}
}

// Handle frees a slot in a game if the caller is the game's host, and asks the
// transport to force-disconnect the removed player.
func (h *Handler) Handle(req actions.Request) (actions.Result, error) {
	return handle(req, h.i.GameService, h.i.SessionService)
}

// handle is Handle over only the two services it uses. The seam keeps the rest
// of the injector — and the database every other service it holds needs — out
// of the way, so a failing removal can be driven through a real game store.
func handle(req actions.Request, games games, sessions sessions) (actions.Result, error) {
	if req.Session == nil {
		return actions.Result{}, fmt.Errorf("must be logged in to kick a player")
	}

	gameCode, err := handlerUtils.RequireGameCode(req.Payload)
	if err != nil {
		return actions.Result{}, err
	}

	targetSlotId, ok := req.Payload["slotId"].(string)
	if !ok || targetSlotId == "" {
		return actions.Result{}, fmt.Errorf("slotId to kick must be provided")
	}

	g, err := games.GetByCode(*gameCode)
	if err != nil {
		return actions.Result{}, err
	}

	// The caller is authorized from their own session, never from the
	// client-supplied target.
	if err := handlerUtils.RequireHost(req.Session, g); err != nil {
		return actions.Result{}, err
	}

	gameId := g.ID

	target, ok := g.Players[targetSlotId]
	if !ok || target.UserID == "" {
		return actions.Result{}, fmt.Errorf("slot %v is empty or not found in game %v", targetSlotId, *gameCode)
	}

	// Free the slot whatever the target's session says: a player can hold a slot
	// here while their session names another game, or have no session at all.
	// The host check is made again inside the write, against the game as it is
	// committed, so a caller who stopped hosting since the check above cannot
	// still kick.
	//
	// A failed removal must not disconnect anybody: the target would still hold
	// the slot, cut off from the transport and unable to rejoin cleanly or play.
	stillHost := func(g *models.Game) error { return handlerUtils.RequireHost(req.Session, g) }

	if err := games.RemovePlayerIf(gameId, targetSlotId, target.UserID, stillHost); err != nil {
		return actions.Result{}, fmt.Errorf("freeing slot %v of game %v: %w", targetSlotId, gameId, err)
	}

	// The slot is free, so the kick has happened. What remains is closing the
	// target's connection, and only a target whose session is in this game has
	// one to close here: one playing in another game stays connected to it.
	targetSession, err := sessions.GetByUserID(target.UserID)
	if err != nil {
		log.Printf("kick: no session to disconnect for player %v of game %v: %v\n", target.UserID, gameId, err)

		return actions.Result{}, nil
	}

	inThisGame := targetSession.GameID != nil && *targetSession.GameID == gameId

	// Out of the game before the disconnect, so a reconnect with their token
	// cannot bring them back into it. LeaveGame only acts on this game, so a
	// target playing elsewhere keeps their session there.
	if err := sessions.LeaveGame(targetSession.ID, gameId); err != nil {
		log.Printf("kick: could not take session %v out of game %v: %v\n", targetSession.ID, gameId, err)
	}

	if !inThisGame {
		return actions.Result{}, nil
	}

	return actions.Result{DisconnectIDs: []string{targetSession.ID}}, nil
}
