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

	sessionRepo "github.com/robbiebyrd/indri/internal/repo/session"
	userRepo "github.com/robbiebyrd/indri/internal/repo/user"
	"github.com/robbiebyrd/indri/internal/services/connection"
)

type Service struct {
	m  *melody.Melody
	ur *userRepo.Store
	sr *sessionRepo.Store
}

// NewService creates a new repository for accessing user data.
func NewService(ctx context.Context, m *melody.Melody, userRepo *userRepo.Store, sessionRepo *sessionRepo.Store) (*Service, error) {
	if ctx == nil {
		return nil, errors.New("context was not passed to the connection service")
	}

	if m == nil {
		return nil, errors.New("melody client was not passed to the connection service")
	}

	if userRepo == nil {
		return nil, errors.New("user repo was not passed to the connection service")
	}

	if sessionRepo == nil {
		return nil, errors.New("session repo was not passed to the connection service")
	}

	return &Service{m, userRepo, sessionRepo}, nil
}

func (bs *Service) Broadcast(gameId *string, teamId *string, data interface{}) error {
	if gameId == nil {
		return errors.New("game id is required")
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	if teamId != nil {
		return bs.sendToTeam(*gameId, *teamId, jsonData)
	}

	return bs.sendToGame(*gameId, jsonData)
}

func (bs *Service) BroadcastToPlayer(gameId *string, data interface{}, playerId string) error {
	if gameId == nil {
		return errors.New("game id is required")
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	return bs.sendToPlayer(*gameId, playerId, jsonData)
}

func (bs *Service) BroadcastToPlayers(gameId *string, data interface{}, playerIds ...string) error {
	if gameId == nil {
		return errors.New("game id is required")
	}

	sort.Strings(playerIds)

	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	return bs.sendToPlayers(*gameId, playerIds, jsonData)
}

func (bs *Service) BroadcastToAll(data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	return bs.sendToAll(jsonData)
}

func (bs *Service) sendToGame(gameId string, jsonData []byte) error {
	log.Printf("Broadcasting to game %v\n", gameId)

	sessions, err := bs.sr.Find("gameId", gameId)
	if err != nil {
		return err
	}

	sessionIds := make([]string, len(sessions))
	for i, session := range sessions {
		sessionIds[i] = session.ID.Hex()
	}

	if err := bs.m.BroadcastFilter(jsonData, func(s *melody.Session) bool {
		if thisGameId, ok := s.Get("sessionId"); ok {
			return slices.Contains(sessionIds, thisGameId.(string))
		}

		return false
	}); err != nil {
		return err
	}

	return nil
}

func (bs *Service) sendToTeam(gameId, teamId string, jsonData []byte) error {
	log.Printf("Broadcasting to game %v and team %v\n", gameId, teamId)

	sessions, err := bs.sr.Find("gameId", gameId)
	if err != nil {
		return err
	}

	var sessionIds []string

	for _, session := range sessions {
		sessionIds = append(sessionIds, session.ID.Hex())
	}

	if err := bs.m.BroadcastFilter(jsonData, func(s *melody.Session) bool {
		cs := connection.NewService(s, bs.m)
		thisSessionId, err := cs.GetKeyAsString("sessionId")
		if err != nil {
			return false
		}
		return err != nil && slices.Contains(sessionIds, *thisSessionId)
	}); err != nil {
		return err
	}

	return nil
}

func (bs *Service) sendToAll(jsonData []byte) error {
	log.Printf("Broadcasting to all\n")

	err := bs.m.Broadcast(jsonData)
	if err != nil {
		return err
	}

	return nil
}

func (bs *Service) sendToPlayer(gameId, playerId string, jsonData []byte) error {
	log.Printf("Broadcasting to game %v and player %v\n", gameId, playerId)

	if _, err := bs.ur.Get(playerId); err != nil {
		return fmt.Errorf("could not get player %v: %v", playerId, err)
	}

	return bs.m.BroadcastFilter(jsonData, func(s *melody.Session) bool {
		thisGameId, ok1 := s.Get("gameId")
		thisPlayerId, ok2 := s.Get("userId")

		return ok1 && ok2 && thisGameId == gameId && thisPlayerId == playerId
	})
}

func (bs *Service) sendToPlayers(gameId string, playerIds []string, jsonData []byte) error {
	log.Printf("Broadcasting to game %v and players %v\n", gameId, playerIds)

	for _, playerId := range playerIds {
		if _, err := bs.ur.Get(playerId); err != nil {
			return fmt.Errorf("could not get player %v: %v", playerId, err)
		}
	}

	return bs.m.BroadcastFilter(jsonData, func(s *melody.Session) bool {
		thisGameId, ok1 := s.Get("code")
		thisPlayerId, ok2 := s.Get("userId")

		return ok1 && ok2 && thisGameId == gameId && slices.Contains(playerIds, thisPlayerId.(string))
	})
}
