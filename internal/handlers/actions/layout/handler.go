package layout

import (
	"context"
	"fmt"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
	handlerUtils "github.com/robbiebyrd/indri/internal/handlers/utils"
	"github.com/robbiebyrd/indri/internal/injector"
	"github.com/robbiebyrd/indri/internal/models"
)

// gameLookup resolves the target game from the code on the wire. Narrow on
// purpose: this handler reads one game and writes one field, so it declares
// exactly that much of the game service.
type gameLookup interface {
	GetByCode(gameCode string) (*models.Game, error)
}

// gameMutator is the read-modify-write path. Mutate bounds its lock wait and
// its database calls with the context it is given, diffs before against after,
// and publishes the delta itself on commit — which is why this handler never
// publishes and never fills Result.Responses.
type gameMutator interface {
	Mutate(ctx context.Context, id string, apply func(g *models.Game) error) error
}

type Handler struct {
	i *injector.Injector
}

func New(i *injector.Injector) *Handler {
	return &Handler{i}
}

// Handle applies one host-authored layout edit to a game.
//
// It resolves its dependencies from the injector here rather than in New
// because registerHandlers is exercised with a partial injector (no repos, no
// services) by the registry coverage test, and a constructor that dereferenced
// those would panic there.
func (h *Handler) Handle(req actions.Request) (actions.Result, error) {
	return editLayout(req, h.i.GameService, h.i.GameRepo)
}

// editLayout is the whole action, over the two capabilities it needs.
//
// The caller is taken from req.Session and from nowhere else. The transport
// authenticated and resolved that session before dispatch, so there is no code
// path by which a userId in the payload can become the caller — payload fields
// are only ever the *subject* of an edit, never its author. Checks run in the
// order of the plan's authorization matrix: authenticated, then in the game,
// then host, and only then is the op decoded, because a caller who may not
// edit should learn nothing about which ops exist.
func editLayout(req actions.Request, games gameLookup, mutator gameMutator) (actions.Result, error) {
	if req.Session == nil {
		return actions.Result{}, fmt.Errorf("must be logged in to edit a layout")
	}

	callerSession := req.Session
	if callerSession.UserID == nil {
		return actions.Result{}, fmt.Errorf("calling session has no user id")
	}

	gameCode, err := handlerUtils.RequireGameCode(req.Payload)
	if err != nil {
		return actions.Result{}, err
	}

	g, err := games.GetByCode(*gameCode)
	if err != nil {
		return actions.Result{}, err
	}

	gameId := g.ID

	if callerSession.GameID == nil || *callerSession.GameID != gameId {
		return actions.Result{}, fmt.Errorf("caller %v is not in game %v", *callerSession.UserID, *gameCode)
	}

	if !g.Players[*callerSession.UserID].Host {
		return actions.Result{}, fmt.Errorf("caller %v is not the host of game %v", *callerSession.UserID, *gameCode)
	}

	op, err := decodeOp(req.Payload)
	if err != nil {
		return actions.Result{}, err
	}

	// The published delta is the response: every player in the game, the
	// editing host included, learns about the edit through the broadcast.
	// Nothing goes back to the caller directly.
	if err := mutator.Mutate(req.Ctx(), gameId, func(g *models.Game) error {
		return applyLayoutOp(g, op)
	}); err != nil {
		return actions.Result{}, fmt.Errorf("editing the layout of game %v: %w", *gameCode, err)
	}

	return actions.Result{}, nil
}
