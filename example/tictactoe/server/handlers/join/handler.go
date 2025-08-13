package move

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/olahol/melody"

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

	gameId, _, err := h.i.SessionService.GetGameIDAndTeamID(*sessionId)
	if err != nil {
		return err
	}

	g, err := h.i.GameService.Get(*gameId)
	if err != nil {
		return err
	}

	sceneData := g.Stage.Scenes[g.Stage.CurrentScene].PublicData
	if sceneData == nil {
		return fmt.Errorf("sceneData is nil")
	}

	updateSceneData := *sceneData
	moveCopy := *move
	boardData := updateSceneData["board"].([][]string)

	a := boardData[moveCopy[0]][moveCopy[1]]
	if a != "" {
		return fmt.Errorf("spot is taken")
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
			return nil, fmt.Errorf("Error converting string '%s' to int: %v\n", s, err)
		} else if num < 0 || num > 2 {
			return nil, fmt.Errorf("invalid move: %v\n", moveString)
		}

		move = append(move, num)
	}

	if len(move) != 2 {
		return nil, fmt.Errorf("invalid move: %v", moveString)
	}

	return &move, nil
}
