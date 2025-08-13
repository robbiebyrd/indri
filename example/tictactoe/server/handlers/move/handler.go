package move

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/olahol/melody"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/robbiebyrd/indri/internal/injector"
	"github.com/robbiebyrd/indri/internal/services/connection"
)

type Handler struct {
	i *injector.Injector
}

func New(i *injector.Injector) *Handler {
	return &Handler{i}
}

// Handle processes a join game request, and adds a player to a game.
func (h *Handler) Handle(
	s *melody.Session,
	decodedMsg map[string]interface{},
) error {
	cs := connection.NewService(s, h.i.MelodyClient)

	move, err := h.decodeMove(decodedMsg)
	if err != nil {
		return err
	}

	sessionId, err := cs.GetKeyAsString("sessionId")
	if err != nil {
		return err
	} else if sessionId == nil {
		return fmt.Errorf("sessionId is nil")
	}

	gameId, teamId, err := h.i.SessionService.GetGameIDAndTeamID(*sessionId)
	if err != nil {
		return err
	}

	g, err := h.i.GameService.Get(*gameId)
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
	moveCopy := *move

	var boardData [][]string

	for _, item := range updateSceneData["board"].(bson.A) {
		var innerRow []string

		for _, item2 := range item.(bson.A) {
			fmt.Println(item2)
			fmt.Printf("%T\n", item2)
			innerRow = append(innerRow, item2.(string))
		}

		boardData = append(boardData, innerRow)
	}

	a := boardData[moveCopy[0]][moveCopy[1]]
	if a != "" {
		return fmt.Errorf("spot is taken")
	}

	boardData[moveCopy[0]][moveCopy[1]] = marker.(string)
	updateSceneData["board"] = boardData
	sceneData.PublicData = &updateSceneData
	g.Stage.Scenes[g.Stage.CurrentScene] = sceneData

	err = h.i.GameRepo.UpdateField(*gameId, "stage", g.Stage)
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

	err = h.i.GameRepo.UpdateField(*gameId, "teams", g.Teams)
	if err != nil {
		return err
	}

	return nil
}

func (h *Handler) decodeMove(decodedMsg map[string]interface{}) (*[]int, error) {
	moveString, ok := decodedMsg["move"]
	if !ok {
		return nil, fmt.Errorf("move is nil")
	}

	moveStrings := strings.Split(moveString.(string), ",")
	move := make([]int, 0, len(moveStrings))

	for _, s := range moveStrings {
		num, err := strconv.Atoi(s)
		if err != nil {
			return nil, fmt.Errorf("error converting string '%s' to int: %v", s, err)
		} else if num < 0 || num > 2 {
			return nil, fmt.Errorf("invalid move: %v", moveString)
		}

		move = append(move, num)
	}

	if len(move) != 2 {
		return nil, fmt.Errorf("invalid move: %v", moveString)
	}

	return &move, nil
}
