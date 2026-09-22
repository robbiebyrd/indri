package luahandler

import (
	"fmt"

	lua "github.com/yuin/gopher-lua"

	"github.com/robbiebyrd/indri/internal/injector"
	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/services/connection"
	"github.com/robbiebyrd/indri/internal/transport"
)

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

	return h.i.GameRepo.Mutate(gameId, func(g *models.Game) error {
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

		if err := L.DoString(h.script); err != nil {
			return fmt.Errorf("running handler script: %w", err)
		}

		return applyLuaToGame(L, gameTable, g)
	})
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
	if sessionId == nil {
		return "", "", "", fmt.Errorf("sessionId is nil")
	}

	session, err := h.i.SessionService.Get(*sessionId)
	if err != nil {
		return "", "", "", err
	}
	if session.GameID == nil || session.TeamID == nil || session.UserID == nil {
		return "", "", "", fmt.Errorf("session is not in a game/team")
	}

	return *session.GameID, *session.TeamID, *session.UserID, nil
}
