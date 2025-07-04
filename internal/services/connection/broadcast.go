package connection

import (
	"encoding/json"
	"errors"
	"github.com/olahol/melody"
	"github.com/robbiebyrd/indri/internal/services/session"
	"log"
)

func (cs *Service) Broadcast(gameCode *string, teamId *string, data interface{}) error {
	if gameCode == nil {
		return errors.New("game id is required")
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	if teamId != nil {
		return cs.broadcastToTeam(*gameCode, *teamId, jsonData)
	}

	return cs.broadcastToGame(*gameCode, jsonData)
}

func (cs *Service) BroadcastAll(data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	return cs.broadcastToAll(jsonData)
}

func (cs *Service) broadcastToGame(gameCode string, jsonData []byte) error {
	log.Printf("Broadcasting to game %v\n", gameCode)

	if err := cs.m.BroadcastFilter(jsonData, func(s *melody.Session) bool {
		thisGameCode, ok := s.Get("code")
		if !ok {
			return false
		}

		return thisGameCode == gameCode
	}); err != nil {
		return err
	}

	return nil
}

func (cs *Service) broadcastToTeam(gameCode string, teamId string, jsonData []byte) error {
	log.Printf("Broadcasting to game %v and team %v\n", gameCode, teamId)

	if err := cs.m.BroadcastFilter(jsonData, func(s *melody.Session) bool {
		ss := session.NewService(s)
		thisGameCode, thisTeamId, _, err := ss.GetStandardKeys()
		return err != nil && *thisGameCode == gameCode && *thisTeamId == teamId
	}); err != nil {
		return err
	}

	return nil
}

func (cs *Service) broadcastToAll(jsonData []byte) error {
	log.Printf("Broadcasting to all\n")

	err := cs.m.Broadcast(jsonData)
	if err != nil {
		return err
	}

	return nil
}
