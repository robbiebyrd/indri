package kick

import (
	"fmt"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
	handlerUtils "github.com/robbiebyrd/indri/internal/handlers/utils"
	"github.com/robbiebyrd/indri/internal/injector"
	"github.com/robbiebyrd/indri/internal/models"
)

// games is the narrow view of the game service a kick needs: find the game the
// caller named, and remove a player from it.
type games interface {
	GetByCode(gameCode string) (*models.Game, error)
	RemovePlayer(id string, userId string) error
}

// sessions is the narrow view of the session service a kick needs: resolve the
// target's session, which carries both the game the target is in and the id the
// transport disconnects.
type sessions interface {
	GetByUserID(userId string) (*models.Session, error)
}

type Handler struct {
	i *injector.Injector
}

func New(i *injector.Injector) *Handler {
	return &Handler{i}
}

// Handle removes a player from a game if the caller is the game's host, and
// asks the transport to force-disconnect the removed player.
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

	callerSession := req.Session
	if callerSession.UserID == nil {
		return actions.Result{}, fmt.Errorf("calling session has no user id")
	}

	gameCode, err := handlerUtils.RequireGameCode(req.Payload)
	if err != nil {
		return actions.Result{}, err
	}

	targetUserId, ok := req.Payload["userId"].(string)
	if !ok || targetUserId == "" {
		return actions.Result{}, fmt.Errorf("userId to kick must be provided")
	}

	g, err := games.GetByCode(*gameCode)
	if err != nil {
		return actions.Result{}, err
	}

	gameId := g.ID.Hex()

	if callerSession.GameID == nil || *callerSession.GameID != gameId {
		return actions.Result{}, fmt.Errorf("caller %v is not in game %v", *callerSession.UserID, *gameCode)
	}

	if !g.Players[*callerSession.UserID].Host {
		return actions.Result{}, fmt.Errorf("caller %v is not the host of game %v", *callerSession.UserID, *gameCode)
	}

	targetSession, err := sessions.GetByUserID(targetUserId)
	if err != nil {
		return actions.Result{}, err
	}

	if targetSession.GameID == nil || *targetSession.GameID != gameId {
		return actions.Result{}, fmt.Errorf("target %v is not in game %v", targetUserId, *gameCode)
	}

	// A failed removal must not disconnect anybody: the target would still be a
	// player of record in the game, cut off from the transport and unable to
	// rejoin cleanly or play.
	if err := games.RemovePlayer(gameId, targetUserId); err != nil {
		return actions.Result{}, fmt.Errorf("removing player %v from game %v: %w", targetUserId, gameId, err)
	}

	// The removal committed, so the target is no longer a player of the game.
	// The transport force-disconnects them if they are currently connected; an
	// offline target simply has no connection to close.
	return actions.Result{DisconnectIDs: []string{targetSession.ID.Hex()}}, nil
}
