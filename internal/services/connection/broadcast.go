package connection

import (
	"encoding/json"
	"errors"
	"github.com/olahol/melody"
	"indri/internal/services/session"
	"log"
)

func Broadcast(m *melody.Melody, gameId *string, teamId *string, data interface{}) error {

	if gameId == nil {
		return errors.New("game id is required")
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	if teamId != nil {
		return broadcastToTeam(m, *gameId, *teamId, jsonData)
	}

	return broadcastToGame(m, *gameId, jsonData)
}

func BroadcastAll(m *melody.Melody, data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	return broadcastToAll(m, jsonData)
}

func broadcastToGame(m *melody.Melody, gameId string, jsonData []byte) error {
	log.Printf("Broadcasting to game %v\n", gameId)

	if err := m.BroadcastFilter(jsonData, func(s *melody.Session) bool {
		ss := session.NewService(s)
		thisGameId, _ := ss.GetKeyAsString("gameId")
		return *thisGameId == gameId
	}); err != nil {
		return err
	}

	return nil
}

func broadcastToTeam(m *melody.Melody, gameId string, teamId string, jsonData []byte) error {
	log.Printf("Broadcasting to game %v and team %v\n", gameId, teamId)

	if err := m.BroadcastFilter(jsonData, func(s *melody.Session) bool {
		ss := session.NewService(s)
		thisGameId, thisTeamId, _, err := ss.GetStandardKeys()
		return err != nil && *thisGameId == gameId && *thisTeamId == teamId
	}); err != nil {
		return err
	}

	return nil
}

func broadcastToAll(m *melody.Melody, jsonData []byte) error {
	log.Printf("Broadcasting to all\n")

	err := m.Broadcast(jsonData)
	if err != nil {
		return err
	}

	return nil
}
