package broadcast

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"slices"
	"sort"

	"github.com/olahol/melody"

	"github.com/robbiebyrd/indri/internal/models"
	sessionRepo "github.com/robbiebyrd/indri/internal/repo/session"
	userRepo "github.com/robbiebyrd/indri/internal/repo/user"
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

	return bs.broadcastToSessions(sessionIDs(sessions), jsonData)
}

func (bs *Service) sendToTeam(gameId, teamId string, jsonData []byte) error {
	log.Printf("Broadcasting to game %v and team %v\n", gameId, teamId)

	sessions, err := bs.sr.Find("gameId", gameId)
	if err != nil {
		return err
	}

	var ids []string

	for _, session := range sessions {
		if session.TeamID != nil && *session.TeamID == teamId {
			ids = append(ids, session.ID.Hex())
		}
	}

	return bs.broadcastToSessions(ids, jsonData)
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

	sessions, err := bs.sr.Find("userId", playerId)
	if err != nil {
		return err
	}

	return bs.broadcastToSessions(sessionsInGame(sessions, gameId), jsonData)
}

func (bs *Service) sendToPlayers(gameId string, playerIds []string, jsonData []byte) error {
	log.Printf("Broadcasting to game %v and players %v\n", gameId, playerIds)

	var ids []string

	for _, playerId := range playerIds {
		sessions, err := bs.sr.Find("userId", playerId)
		if err != nil {
			return err
		}

		ids = append(ids, sessionsInGame(sessions, gameId)...)
	}

	return bs.broadcastToSessions(ids, jsonData)
}

// broadcastToSessions sends jsonData to the melody connections whose
// "sessionId" key is in sessionIds. "sessionId" (the session ObjectID) is the
// only per-connection key the app sets, so all targeted sends resolve their
// recipients through the session store and match on it.
func (bs *Service) broadcastToSessions(sessionIds []string, jsonData []byte) error {
	if len(sessionIds) == 0 {
		return nil
	}

	return bs.m.BroadcastFilter(jsonData, func(s *melody.Session) bool {
		value, ok := s.Get("sessionId")
		if !ok {
			return false
		}

		id, ok := value.(string)

		return ok && slices.Contains(sessionIds, id)
	})
}

// sessionIDs returns the hex ids of the given sessions.
func sessionIDs(sessions []*models.Session) []string {
	ids := make([]string, len(sessions))
	for i, session := range sessions {
		ids[i] = session.ID.Hex()
	}

	return ids
}

// sessionsInGame returns the hex ids of the sessions currently in gameId.
func sessionsInGame(sessions []*models.Session, gameId string) []string {
	var ids []string

	for _, session := range sessions {
		if session.GameID != nil && *session.GameID == gameId {
			ids = append(ids, session.ID.Hex())
		}
	}

	return ids
}
