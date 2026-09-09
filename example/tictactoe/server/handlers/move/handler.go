package move

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/olahol/melody"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/robbiebyrd/indri/internal/injector"
	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/services/connection"
)

type TicTacToeMoveHandler struct {
	i *injector.Injector
}

func New(i *injector.Injector) *TicTacToeMoveHandler {
	return &TicTacToeMoveHandler{i}
}

func (h *TicTacToeMoveHandler) getGameDataFromSession(s *melody.Session) (*models.Game, *string, error) {
	cs := connection.NewService(s, h.i.MelodyClient)

	sessionId, err := cs.GetKeyAsString("sessionId")
	if err != nil {
		return nil, nil, err
	} else if sessionId == nil {
		return nil, nil, fmt.Errorf("sessionId is nil")
	}

	gameId, teamId, err := h.i.SessionService.GetGameIDAndTeamID(*sessionId)
	if err != nil {
		return nil, nil, err
	}

	g, err := h.i.GameService.Get(*gameId)
	if err != nil {
		return nil, nil, err
	}

	return g, teamId, nil
}

func (h *TicTacToeMoveHandler) findTeamByMarker(marker string, g *models.Game) (*string, error) {
	for tId, t := range g.Teams {
		if t.PublicData["marker"] == marker {
			return &tId, nil
		}
	}
	return nil, fmt.Errorf("no team found with marker %s", marker)
}

// Handle a player's move request.
func (h *TicTacToeMoveHandler) Handle(
	s *melody.Session,
	decodedMsg map[string]interface{},
) error {
	g, teamId, err := h.getGameDataFromSession(s)
	if err != nil {
		return err
	}

	thisTeam := g.Teams[*teamId]

	marker, ok := thisTeam.PublicData["marker"]
	if !ok {
		return fmt.Errorf("marker is nil")
	}

	isPlayerTurn, ok := thisTeam.PublicData["turn"]
	if !ok {
		return fmt.Errorf("marker is nil")
	} else if !isPlayerTurn.(bool) {
		return fmt.Errorf("it is not your turn")
	}

	sceneData := g.Stage.Scenes[g.Stage.CurrentScene]

	updateSceneData := *sceneData.PublicData

	var boardData [][]string

	for _, item := range updateSceneData["board"].(bson.A) {
		var innerRow []string

		for _, item2 := range item.(bson.A) {
			innerRow = append(innerRow, item2.(string))
		}

		boardData = append(boardData, innerRow)
	}

	rows, columns := getBoardSize(boardData)

	move, err := h.decodeMove(decodedMsg, columns, rows)
	if err != nil {
		return err
	}

	moveCopy := *move

	spot := boardData[moveCopy[0]][moveCopy[1]]
	if spot != "" {
		return fmt.Errorf("spot is taken")
	}

	boardData[moveCopy[0]][moveCopy[1]] = marker.(string)

	winner, won := h.findWinner(boardData)
	if won {
		if winner == "draw" {
			updateSceneData["winningTeam"] = "draw"
		} else {
			winningTeam, err := h.findTeamByMarker(winner, g)
			if err != nil {
				return err
			}
			updateSceneData["winningTeam"] = winningTeam
		}
	}

	updateSceneData["board"] = boardData
	sceneData.PublicData = &updateSceneData
	g.Stage.Scenes[g.Stage.CurrentScene] = sceneData

	err = h.i.GameRepo.UpdateField(g.ID.Hex(), "stage", g.Stage)
	if err != nil {
		return err
	}

	teams := g.Teams
	for tId, t := range teams {
		if tId == *teamId {
			t.PublicData["turn"] = false
		} else {
			t.PublicData["turn"] = true
		}
	}

	g.Teams = teams

	err = h.i.GameRepo.UpdateField(g.ID.Hex(), "teams", g.Teams)
	if err != nil {
		return err
	}

	return nil
}

func (h *TicTacToeMoveHandler) decodeMove(decodedMsg map[string]interface{}, columns, rows int) (*[]int, error) {
	moveString, ok := decodedMsg["move"]
	if !ok {
		return nil, fmt.Errorf("move is nil")
	}

	moveStrings := strings.Split(moveString.(string), ",")
	move := make([]int, 0, len(moveStrings))

	for _, s := range moveStrings {
		num, err := strconv.Atoi(s)
		if err != nil {
			return nil, fmt.Errorf("error converting string '%s' to int: %w", s, err)
		}

		move = append(move, num)
	}

	if len(move) != 2 {
		return nil, fmt.Errorf("invalid move: %v", moveString)
	}

	if move[0] < 0 || move[0] >= rows || move[1] < 0 || move[1] >= columns {
		return nil, fmt.Errorf("move out of bounds: %v", moveString)
	}

	return &move, nil
}

// getBoardSize returns the number of rows and columns on the board.
func getBoardSize(boardData [][]string) (rows, columns int) {
	if len(boardData) == 0 {
		return 0, 0
	}

	return len(boardData), len(boardData[0])
}

func checkStraightAcrossWin(boardData [][]string, marker string) bool {
	rows, columns := getBoardSize(boardData)

	// Check each row
	for i := range rows {
		win := true
		for j := range columns {
			if boardData[i][j] != marker {
				win = false
				break
			}
		}
		if win {
			return true
		}
	}

	// Check each column
	for j := range columns {
		win := true
		for i := range rows {
			if boardData[i][j] != marker {
				win = false
				break
			}
		}
		if win {
			return true
		}
	}

	return false
}

func checkDiagonalWin(boardData [][]string, marker string) bool {
	// Check for diagonal wins
	rows, columns := getBoardSize(boardData)
	for sum := 0; sum < rows+columns-1; sum++ {
		count := 0
		for rowCount := range rows {
			columnCount := sum - rowCount
			if columnCount >= 0 && columnCount < columns {
				if boardData[rowCount][columnCount] == marker {
					count++
				}
			}
		}
		if count == rows {
			return true
		}
	}

	// Check for anti-diagonal wins
	for diff := -(rows - 1); diff < columns; diff++ {
		count := 0
		for rowCount := range rows {
			columnCount := rowCount - diff
			if columnCount >= 0 && columnCount < columns {
				if boardData[rowCount][columnCount] == marker {
					count++
				}
			}
		}
		if count == rows {
			return true
		}
	}

	return false
}

func (h *TicTacToeMoveHandler) findWinner(boardData [][]string) (string, bool) {
	// Fetch all the markers that have been placed on the board.
	markers := h.getUniqueStrings(boardData, false)

	if len(markers) < 2 {
		// Both players must place at least one marker before a win can occur
		return "", false
	}

	for _, marker := range markers {
		// Check for column wins
		winner := checkStraightAcrossWin(boardData, marker)
		if winner {
			return marker, true
		}
		winner = checkDiagonalWin(boardData, marker)
		if winner {
			return marker, true
		}
	}

	if slices.Contains(h.getUniqueStrings(boardData, true), "") {
		return "", false
	}

	return "draw", true
}

func (h *TicTacToeMoveHandler) getUniqueStrings(data [][]string, includeEmpty bool) []string {
	unique := make(map[string]bool)
	for _, row := range data {
		for _, s := range row {
			if !includeEmpty && s == "" {
				continue
			}
			unique[s] = true
		}
	}
	result := make([]string, 0, len(unique))
	for s := range unique {
		result = append(result, s)
	}
	return result
}
