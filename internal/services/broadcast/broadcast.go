package broadcast

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"slices"
	"sort"

	"github.com/olahol/melody"

	userRepo "github.com/robbiebyrd/indri/internal/repo/user"
	"github.com/robbiebyrd/indri/internal/services/connection"
)

type Service struct {
	m  *melody.Melody
	ur *userRepo.Repo
}

// NewService creates a new repository for accessing user data.
func NewService(ctx context.Context, m *melody.Melody, userRepo *userRepo.Repo) (*Service, error) {
	if ctx == nil {
		return nil, errors.New("context was not passed to the connection service")
	}

	if m == nil {
		return nil, errors.New("melody client was not passed to the connection service")
	}

	if userRepo == nil {
		return nil, errors.New("user repo was not passed to the connection service")
	}

	return &Service{m, userRepo}, nil
}

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
		ss := connection.NewService(s, cs.m)
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
