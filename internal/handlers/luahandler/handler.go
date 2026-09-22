package luahandler

import (
	"encoding/json"
	"fmt"
	"log"

	lua "github.com/yuin/gopher-lua"

	"github.com/robbiebyrd/indri/internal/injector"
	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/services/connection"
	"github.com/robbiebyrd/indri/internal/transport"
)

// postMutateActions records side-effects requested by a Lua script that must
// fire after GameRepo.Mutate commits — never inside the apply callback.
// Allocated before Mutate so the same pointer is shared across retries.
type postMutateActions struct {
	refreshAll  bool
	refreshSelf bool
}

// registerIndriTable registers the indri.refresh() and indri.refreshSelf()
// globals into L. Both functions set flags on pma; the actual network sends
// happen after Mutate returns via executePostMutateActions.
func registerIndriTable(L *lua.LState, pma *postMutateActions) {
	indri := L.NewTable()
	L.SetField(indri, "refresh", L.NewFunction(func(L *lua.LState) int {
		pma.refreshAll = true
		return 0
	}))
	L.SetField(indri, "refreshSelf", L.NewFunction(func(L *lua.LState) int {
		pma.refreshSelf = true
		return 0
	}))
	L.SetGlobal("indri", indri)
}

// executePostMutateActions runs any refresh side-effects requested via the
// indri global. Errors are logged and discarded — the mutation already
// committed, so a fan-out failure must not fail the overall operation.
func executePostMutateActions(pma *postMutateActions, gameId string, s transport.Conn, i *injector.Injector) {
	if !pma.refreshAll && !pma.refreshSelf {
		return
	}

	g, err := i.GameService.Get(gameId)
	if err != nil {
		log.Printf("indri refresh: could not fetch game %v: %v", gameId, err)
		return
	}
	sanitized := i.GameService.Sanitize(g)

	if pma.refreshAll {
		if err := i.BroadcastService.Broadcast(&gameId, nil, sanitized); err != nil {
			log.Printf("indri refresh: broadcast failed for game %v: %v", gameId, err)
		}
	}

	if pma.refreshSelf {
		jsonData, err := json.Marshal(sanitized)
		if err != nil {
			log.Printf("indri refreshSelf: marshal failed: %v", err)
			return
		}
		cs := connection.NewService(s, i.Transport)
		if err := cs.Write(jsonData); err != nil {
			log.Printf("indri refreshSelf: write failed: %v", err)
		}
	}
}

type Handler struct {
	i      *injector.Injector
	script string
}

func New(i *injector.Injector, script string) *Handler {
	return &Handler{i: i, script: script}
}

// buildCallerTable constructs the Lua caller table exposed to every handler script.
func buildCallerTable(L *lua.LState, gameId, teamId, userId string) *lua.LTable {
	t := L.NewTable()
	t.RawSetString("gameId", lua.LString(gameId))
	t.RawSetString("teamId", lua.LString(teamId))
	t.RawSetString("userId", lua.LString(userId))
	return t
}

func (h *Handler) Handle(s transport.Conn, decodedMsg map[string]interface{}) error {
	gameId, teamId, userId, err := h.gameAndTeamFromSession(s)
	if err != nil {
		return err
	}

	pma := &postMutateActions{}

	if err := h.i.GameRepo.Mutate(gameId, func(g *models.Game) error {
		L := lua.NewState()
		defer L.Close()

		if err := L.DoString(stdlib); err != nil {
			return fmt.Errorf("loading stdlib: %w", err)
		}

		gameTable, err := gameToLua(L, g)
		if err != nil {
			return fmt.Errorf("building game table: %w", err)
		}
		L.SetGlobal("game", gameTable)

		msgTable := L.NewTable()
		for k, v := range decodedMsg {
			msgTable.RawSetString(k, jsonToLua(L, v))
		}
		L.SetGlobal("msg", msgTable)

		L.SetGlobal("caller", buildCallerTable(L, gameId, teamId, userId))

		initialTable, err := buildInitialTable(L, h.i.Script)
		if err != nil {
			return fmt.Errorf("building initial table: %w", err)
		}
		L.SetGlobal("initial", initialTable)

		registerIndriTable(L, pma)

		if err := L.DoString(h.script); err != nil {
			return fmt.Errorf("running handler script: %w", err)
		}

		return applyLuaToGame(L, gameTable, g)
	}); err != nil {
		return err
	}

	executePostMutateActions(pma, gameId, s, h.i)
	return nil
}

// gameAndTeamFromSession resolves the game, team, and user for the connection
// that sent the current message. Uses SessionService.Get to read all three
// fields in a single round trip.
func (h *Handler) gameAndTeamFromSession(s transport.Conn) (gameId, teamId, userId string, err error) {
	cs := connection.NewService(s, h.i.Transport)

	sessionId, err := cs.GetKeyAsString("sessionId")
	if err != nil {
		return "", "", "", err
	}
	session, err := h.i.SessionService.Get(*sessionId)
	if err != nil {
		return "", "", "", err
	}
	if session.UserID == nil {
		return "", "", "", fmt.Errorf("session has no user")
	}
	if session.GameID == nil || session.TeamID == nil {
		return "", "", "", fmt.Errorf("session is not in a game/team")
	}

	return *session.GameID, *session.TeamID, *session.UserID, nil
}
