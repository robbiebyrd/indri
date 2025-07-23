package connection

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/olahol/melody"
	"github.com/robbiebyrd/indri/internal/services/session"
	"log"
	"slices"
	"sort"
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
		return cs.sendToTeam(*gameCode, *teamId, jsonData)
	}

	return cs.sendToGame(*gameCode, jsonData)
}

func (cs *Service) BroadcastToPlayer(gameCode *string, data interface{}, playerId string) error {
	if gameCode == nil {
		return errors.New("game id is required")
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	return cs.sendToPlayer(*gameCode, playerId, jsonData)
}

func (cs *Service) BroadcastToPlayers(gameCode *string, data interface{}, playerIds ...string) error {
	if gameCode == nil {
		return errors.New("game id is required")
	}

	sort.Strings(playerIds)

	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	return cs.sendToPlayers(*gameCode, playerIds, jsonData)
}

func (cs *Service) BroadcastToAll(data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	return cs.sendToAll(jsonData)
}

func (cs *Service) sendToGame(gameCode string, jsonData []byte) error {
	log.Printf("Broadcasting to game %v\n", gameCode)

	if err := cs.m.BroadcastFilter(jsonData, func(s *melody.Session) bool {
		if thisGameCode, ok := s.Get("code"); ok {
			return thisGameCode == gameCode
		}

		return false
	}); err != nil {
		return err
	}

	return nil
}

func (cs *Service) sendToTeam(gameCode string, teamId string, jsonData []byte) error {
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

func (cs *Service) sendToAll(jsonData []byte) error {
	log.Printf("Broadcasting to all\n")

	err := cs.m.Broadcast(jsonData)
	if err != nil {
		return err
	}

	return nil
}

func (cs *Service) sendToPlayer(gameCode, playerId string, jsonData []byte) error {
	log.Printf("Broadcasting to game %v and player %v\n", gameCode, playerId)

	if _, err := cs.us.Get(playerId); err != nil {
		return fmt.Errorf("could not get player %v: %v", playerId, err)
	}

	return cs.m.BroadcastFilter(jsonData, func(s *melody.Session) bool {
		thisGameCode, ok1 := s.Get("code")
		thisPlayerId, ok2 := s.Get("userId")

		return ok1 && ok2 && thisGameCode == gameCode && thisPlayerId == playerId
	})
}

func (cs *Service) sendToPlayers(gameCode string, playerIds []string, jsonData []byte) error {
	log.Printf("Broadcasting to game %v and players %v\n", gameCode, playerIds)

	for _, playerId := range playerIds {
		if _, err := cs.us.Get(playerId); err != nil {
			return fmt.Errorf("could not get player %v: %v", playerId, err)
		}
	}

	return cs.m.BroadcastFilter(jsonData, func(s *melody.Session) bool {
		thisGameCode, ok1 := s.Get("code")
		thisPlayerId, ok2 := s.Get("userId")

		return ok1 && ok2 && thisGameCode == gameCode && slices.Contains(playerIds, thisPlayerId.(string))
	})
}
