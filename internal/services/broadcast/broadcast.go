package broadcast

import (
	"context"
	"errors"
	"log"
	"slices"
	"sort"

	"github.com/robbiebyrd/indri/internal/models"
	sessionRepo "github.com/robbiebyrd/indri/internal/repo/session"
	userRepo "github.com/robbiebyrd/indri/internal/repo/user"
	"github.com/robbiebyrd/indri/internal/transport"
)

type Service struct {
	t  transport.Transport
	ur *userRepo.Store
	sr *sessionRepo.Store
}

// NewService creates a new service for broadcasting to connected clients.
func NewService(ctx context.Context, t transport.Transport, userRepo *userRepo.Store, sessionRepo *sessionRepo.Store) (*Service, error) {
	if ctx == nil {
		return nil, errors.New("context was not passed to the connection service")
	}

	if t == nil {
		return nil, errors.New("transport was not passed to the connection service")
	}

	if userRepo == nil {
		return nil, errors.New("user repo was not passed to the connection service")
	}

	if sessionRepo == nil {
		return nil, errors.New("session repo was not passed to the connection service")
	}

	return &Service{t, userRepo, sessionRepo}, nil
}

func (bs *Service) Broadcast(gameId *string, teamId *string, data interface{}) error {
	if gameId == nil {
		return errors.New("game id is required")
	}

	if teamId != nil {
		return bs.sendToTeam(*gameId, *teamId, data)
	}

	return bs.sendToGame(*gameId, data)
}

func (bs *Service) BroadcastToPlayer(gameId *string, data interface{}, playerId string) error {
	if gameId == nil {
		return errors.New("game id is required")
	}

	return bs.sendToPlayer(*gameId, playerId, data)
}

func (bs *Service) BroadcastToPlayers(gameId *string, data interface{}, playerIds ...string) error {
	if gameId == nil {
		return errors.New("game id is required")
	}

	sort.Strings(playerIds)

	return bs.sendToPlayers(*gameId, playerIds, data)
}

func (bs *Service) sendToGame(gameId string, payload interface{}) error {
	log.Printf("Broadcasting to game %v\n", gameId)

	sessions, err := bs.sr.Find("gameId", gameId)
	if err != nil {
		return err
	}

	return bs.broadcastToSessions(sessionIDs(sessions), payload)
}

func (bs *Service) sendToTeam(gameId, teamId string, payload interface{}) error {
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

	return bs.broadcastToSessions(ids, payload)
}

func (bs *Service) sendToPlayer(gameId, playerId string, payload interface{}) error {
	log.Printf("Broadcasting to game %v and player %v\n", gameId, playerId)

	sessions, err := bs.sr.Find("userId", playerId)
	if err != nil {
		return err
	}

	return bs.broadcastToSessions(sessionsInGame(sessions, gameId), payload)
}

func (bs *Service) sendToPlayers(gameId string, playerIds []string, payload interface{}) error {
	log.Printf("Broadcasting to game %v and players %v\n", gameId, playerIds)

	var ids []string

	for _, playerId := range playerIds {
		sessions, err := bs.sr.Find("userId", playerId)
		if err != nil {
			return err
		}

		ids = append(ids, sessionsInGame(sessions, gameId)...)
	}

	return bs.broadcastToSessions(ids, payload)
}

// broadcastToSessions sends payload to connections whose "sessionId" key is in
// sessionIds, encoding per-connection (MessagePack by default, JSON on ?debug=1).
func (bs *Service) broadcastToSessions(sessionIds []string, payload interface{}) error {
	if len(sessionIds) == 0 {
		return nil
	}

	conns, err := bs.t.Conns()
	if err != nil {
		return err
	}

	for _, c := range conns {
		value, ok := c.Get("sessionId")
		if !ok {
			continue
		}
		id, ok := value.(string)
		if !ok || !slices.Contains(sessionIds, id) {
			continue
		}
		if err := transport.WriteEncoded(c, payload); err != nil {
			log.Printf("broadcast write error to session %s: %v", id, err)
		}
	}

	return nil
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
