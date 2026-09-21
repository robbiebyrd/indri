package restart

import (
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/robbiebyrd/indri/internal/injector"
	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/services/connection"
	"github.com/robbiebyrd/indri/internal/transport"
)

type TicTacToeRestartHandler struct {
	i *injector.Injector
}

func New(i *injector.Injector) *TicTacToeRestartHandler {
	return &TicTacToeRestartHandler{i}
}

func buildEmptyBoard(rows, cols int) [][]string {
	board := make([][]string, rows)
	for i := range board {
		board[i] = make([]string, cols)
	}
	return board
}

func (h *TicTacToeRestartHandler) Handle(
	s transport.Conn,
	_ map[string]interface{},
) error {
	cs := connection.NewService(s, h.i.Transport)

	sessionId, err := cs.GetKeyAsString("sessionId")
	if err != nil {
		return err
	} else if sessionId == nil {
		return fmt.Errorf("sessionId is nil")
	}

	gameId, _, err := h.i.SessionService.GetGameIDAndTeamID(*sessionId)
	if err != nil {
		return err
	}
	if gameId == nil {
		return fmt.Errorf("session is not in a game")
	}

	return h.i.GameRepo.Mutate(*gameId, func(g *models.Game) error {
		sceneData := g.Stage.Scenes[g.Stage.CurrentScene]
		updated := *sceneData.PublicData

		rows, cols := boardDimensions(updated)
		updated["board"] = buildEmptyBoard(rows, cols)
		delete(updated, "winningTeam")

		sceneData.PublicData = &updated
		g.Stage.Scenes[g.Stage.CurrentScene] = sceneData

		for tId, t := range g.Teams {
			if scriptTeam, ok := h.i.Script.Teams[tId]; ok {
				t.PublicData["turn"] = scriptTeam.PublicData["turn"]
			} else {
				t.PublicData["turn"] = false
			}
		}

		return nil
	})
}

func boardDimensions(sceneData map[string]interface{}) (rows, cols int) {
	rows, cols = 3, 3
	board, ok := sceneData["board"].(bson.A)
	if !ok || len(board) == 0 {
		return
	}
	rows = len(board)
	if row, ok := board[0].(bson.A); ok {
		cols = len(row)
	}
	return
}
