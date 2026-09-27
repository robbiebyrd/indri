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
	"github.com/robbiebyrd/indri/internal/services/connection"
	"github.com/robbiebyrd/indri/internal/services/events"
	"github.com/robbiebyrd/indri/internal/transport"
)

// Service sends messages to sessions' connections across the cluster. The
// sending instance resolves which sessions a message is for; with a relay
// bus, every instance then writes it to the connections it holds.
type Service struct {
	ctx   context.Context
	t     transport.Transport
	ur    userRepo.Storer
	sr    sessionRepo.Storer
	relay events.Bus[events.Delivery]
}

// NewService creates a new service for broadcasting to connected clients.
// relay carries deliveries to the other instances; nil means this is the only
// instance, and deliveries are written directly.
func NewService(ctx context.Context, t transport.Transport, userRepo userRepo.Storer, sessionRepo sessionRepo.Storer, relay events.Bus[events.Delivery]) (*Service, error) {
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

	return &Service{ctx: ctx, t: t, ur: userRepo, sr: sessionRepo, relay: relay}, nil
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

// BroadcastLocal sends data to a game's players connected to this instance
// only. It is for messages every instance already receives, such as game
// change events, which would otherwise be relayed once per instance.
func (bs *Service) BroadcastLocal(gameId *string, data interface{}) error {
	if gameId == nil {
		return errors.New("game id is required")
	}

	ids, err := bs.gameSessions(*gameId)
	if err != nil {
		return err
	}

	bs.deliverLocal(events.Delivery{SessionIDs: ids, Payload: data})

	return nil
}

// CloseSessions disconnects the sessions' connections on every instance,
// telling each client it was disconnected first.
func (bs *Service) CloseSessions(sessionIds ...string) error {
	return bs.send(events.Delivery{SessionIDs: sessionIds, Close: true})
}

// RelayDeliveries applies deliveries published by any instance to this
// instance's connections until ctx ends. ready, if non-nil, is closed once
// the subscription is live. Without a relay bus it returns immediately.
func (bs *Service) RelayDeliveries(ctx context.Context, ready chan<- struct{}) error {
	if bs.relay == nil {
		if ready != nil {
			close(ready)
		}
		return nil
	}

	deliveries, err := bs.relay.Subscribe(ctx)
	if err != nil {
		return err
	}

	if ready != nil {
		close(ready)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case d, ok := <-deliveries:
			if !ok {
				return nil
			}
			bs.deliverLocal(d)
		}
	}
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

	ids, err := bs.gameSessions(gameId)
	if err != nil {
		return err
	}

	return bs.broadcastToSessions(ids, payload)
}

// gameSessions returns the ids of the sessions in gameId.
func (bs *Service) gameSessions(gameId string) ([]string, error) {
	sessions, err := bs.sr.Find("gameId", gameId)
	if err != nil {
		return nil, err
	}

	return sessionIDs(sessions), nil
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
			ids = append(ids, session.ID)
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

// broadcastToSessions sends payload to the sessions' connections on every
// instance.
func (bs *Service) broadcastToSessions(sessionIds []string, payload interface{}) error {
	if len(sessionIds) == 0 {
		return nil
	}

	return bs.send(events.Delivery{SessionIDs: sessionIds, Payload: payload})
}

// send hands d to every instance: through the relay bus when there is one,
// otherwise straight to this instance's connections.
func (bs *Service) send(d events.Delivery) error {
	if bs.relay == nil {
		bs.deliverLocal(d)
		return nil
	}

	return bs.relay.Publish(bs.ctx, d)
}

// deliverLocal applies d to this instance's connections whose "sessionId" key
// is one of d's sessions: it closes them, or writes the payload encoded per
// connection (MessagePack by default, JSON on ?debug=1).
func (bs *Service) deliverLocal(d events.Delivery) {
	if len(d.SessionIDs) == 0 {
		return
	}

	conns, err := bs.t.Conns()
	if err != nil {
		log.Printf("delivery: listing connections: %v", err)
		return
	}

	for _, c := range conns {
		value, ok := c.Get("sessionId")
		if !ok {
			continue
		}
		id, ok := value.(string)
		if !ok || !slices.Contains(d.SessionIDs, id) {
			continue
		}

		if d.Close {
			connection.Close(c)
			continue
		}

		if err := transport.WriteEncoded(c, d.Payload); err != nil {
			log.Printf("broadcast write error to session %s: %v", id, err)
		}
	}
}

// sessionIDs returns the ids of the given sessions.
func sessionIDs(sessions []*models.Session) []string {
	ids := make([]string, len(sessions))
	for i, session := range sessions {
		ids[i] = session.ID
	}

	return ids
}

// sessionsInGame returns the ids of the sessions currently in gameId.
func sessionsInGame(sessions []*models.Session, gameId string) []string {
	var ids []string

	for _, session := range sessions {
		if session.GameID != nil && *session.GameID == gameId {
			ids = append(ids, session.ID)
		}
	}

	return ids
}
