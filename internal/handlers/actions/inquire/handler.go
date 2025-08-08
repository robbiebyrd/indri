package inquire

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/olahol/melody"

	"github.com/robbiebyrd/indri/internal/injector"
	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/services/connection"
)

type Handler struct {
	i *injector.Injector
}

func New(i *injector.Injector) *Handler {
	return &Handler{i}
}

type TeamInfo struct {
	Name string `json:"name"`
	Full bool   `json:"full"`
}

type GameInfo struct {
	Code  string     `json:"code,omitempty"`
	Full  bool       `json:"full"`
	Teams []TeamInfo `json:"teams,omitempty"`
}

// Handle processes a join game request, and adds a player to a game.
func (h *Handler) Handle(
	s *melody.Session,
	decodedMsg map[string]interface{},
) error {
	var jsonBytes *[]byte

	cs := connection.NewService(s, h.i.MelodyClient)

	_, err := cs.GetKeyAsString("sessionId")
	if err != nil {
		_ = cs.Write([]byte(`{"authenticated": false, "stage": { "currentScene": "login"}`))
		return fmt.Errorf("unable to get userId: %w", err)
	}

	if _, ok := decodedMsg["inquiryType"]; !ok {
		return errors.New("inquiryType not provided")
	}

	if decodedMsg["inquiryType"].(string) == "game" {
		jbs, err := h.handleGameInquiry(decodedMsg)
		if err != nil {
			return err
		}

		jsonBytes = jbs
	}

	if jsonBytes == nil {
		return nil
	}

	cs.Write(*jsonBytes)

	return nil
}

func (h *Handler) getGamesList() ([]*models.Game, error) {
	games, err := h.i.GameService.FindOpen(100)
	if err != nil {
		return nil, err
	}

	return games, nil
}

func (h *Handler) handleGameInquiry(decodedMsg map[string]interface{}) (*[]byte, error) {
	var jsonBytes *[]byte

	games, err := h.getGamesList()
	if err != nil {
		return nil, err
	}

	if _, ok := decodedMsg["inquiry"]; !ok {
		return nil, errors.New("inquiry not provided")
	}

	switch decodedMsg["inquiry"].(string) {
	case "availableGames":
		var gameInfoList = h.createGameInfoList(games)

		jbs, err := json.Marshal(gameInfoList)
		if err != nil {
			return nil, err
		}

		jsonBytes = &jbs
	case "gameInfo":
		if _, ok := decodedMsg["code"]; !ok {
			return nil, errors.New("game code not provided")
		}

		gameCode := decodedMsg["code"].(string)

		game, err := h.i.GameService.GetByCode(gameCode)
		if err != nil {
			return nil, err
		}

		jbs, err := json.Marshal(h.createGameInfo(game))
		if err != nil {
			return nil, err
		}

		jsonBytes = &jbs
	default:
		return nil, nil
	}

	return jsonBytes, nil
}

func (h *Handler) createGameInfoList(games []*models.Game) []GameInfo {
	var gameInfoList []GameInfo

	for _, game := range games {
		gameInfoList = append(gameInfoList, h.createGameInfo(game))
	}

	return gameInfoList
}

func (h *Handler) createGameInfo(game *models.Game) GameInfo {
	var teamsList []TeamInfo

	availableTeamsCount := h.i.Script.Config.MaxTeams

	for _, team := range game.Teams {
		isFull := len(team.PlayerIDs) >= h.i.Script.Config.MaxPlayersPerTeam
		if isFull {
			availableTeamsCount--
		}

		teamsList = append(teamsList, TeamInfo{
			Name: team.Name,
			Full: isFull,
		})
	}

	var teamsFullList []bool
	for _, team := range teamsList {
		teamsFullList = append(teamsFullList, team.Full)
	}

	openTeam := slices.Contains(teamsFullList, false)

	return GameInfo{
		Code:  game.Code,
		Full:  availableTeamsCount <= 0 && !openTeam,
		Teams: teamsList,
	}
}
