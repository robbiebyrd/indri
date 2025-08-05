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

func (cs *Service) Broadcast(gameId *string, teamId *string, data interface{}) error {
	if gameId == nil {
		return errors.New("game id is required")
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	if teamId != nil {
		return cs.sendToTeam(*gameId, *teamId, jsonData)
	}

	return cs.sendToGame(*gameId, jsonData)
}

func (cs *Service) BroadcastToPlayer(gameId *string, data interface{}, playerId string) error {
	if gameId == nil {
		return errors.New("game id is required")
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	return cs.sendToPlayer(*gameId, playerId, jsonData)
}

func (cs *Service) BroadcastToPlayers(gameId *string, data interface{}, playerIds ...string) error {
	if gameId == nil {
		return errors.New("game id is required")
	}

	sort.Strings(playerIds)

	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	return cs.sendToPlayers(*gameId, playerIds, jsonData)
}

func (cs *Service) BroadcastToAll(data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	return cs.sendToAll(jsonData)
}

func (cs *Service) sendToGame(gameId string, jsonData []byte) error {
	log.Printf("Broadcasting to game %v\n", gameId)

	if err := cs.m.BroadcastFilter(jsonData, func(s *melody.Session) bool {
		if thisGameId, ok := s.Get("gameId"); ok {
			return thisGameId == gameId
		}

		return false
	}); err != nil {
		return err
	}

	return nil
}

func (cs *Service) sendToTeam(gameId string, teamId string, jsonData []byte) error {
	log.Printf("Broadcasting to game %v and team %v\n", gameId, teamId)

	if err := cs.m.BroadcastFilter(jsonData, func(s *melody.Session) bool {
		ss := session.NewService(s, cs.m)
		thisGameId, thisTeamId, _, err := ss.GetStandardKeys()
		return err != nil && *thisGameId == gameId && *thisTeamId == teamId
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

func (cs *Service) sendToPlayer(gameId, playerId string, jsonData []byte) error {
	log.Printf("Broadcasting to game %v and player %v\n", gameId, playerId)

	if _, err := cs.ur.Get(playerId); err != nil {
		return fmt.Errorf("could not get player %v: %v", playerId, err)
	}

	return cs.m.BroadcastFilter(jsonData, func(s *melody.Session) bool {
		thisGameId, ok1 := s.Get("gameId")
		thisPlayerId, ok2 := s.Get("userId")

		return ok1 && ok2 && thisGameId == gameId && thisPlayerId == playerId
	})
}

func (cs *Service) sendToPlayers(gameId string, playerIds []string, jsonData []byte) error {
	log.Printf("Broadcasting to game %v and players %v\n", gameId, playerIds)

	for _, playerId := range playerIds {
		if _, err := cs.ur.Get(playerId); err != nil {
			return fmt.Errorf("could not get player %v: %v", playerId, err)
		}
	}

	return cs.m.BroadcastFilter(jsonData, func(s *melody.Session) bool {
		thisGameId, ok1 := s.Get("code")
		thisPlayerId, ok2 := s.Get("userId")

		return ok1 && ok2 && thisGameId == gameId && slices.Contains(playerIds, thisPlayerId.(string))
	})
}
