package session

import (
	"errors"
	"fmt"
	"github.com/olahol/melody"
	melodyClient "github.com/robbiebyrd/indri/internal/clients/melody"
)

type Service struct {
	s *melody.Session
	m *melody.Melody
}

// NewService creates a new repository for accessing game data.
func NewService(s *melody.Session) *Service {
	m := melodyClient.New()
	return &Service{s, m}
}

// GetStandardKeys returns a set of standard session attributes that identify a user in a game.
// StandardKeys include gameCode, teamId, and userId.
func (ss *Service) GetStandardKeys() (*string, *string, *string, error) {
	gameCodePtr, err := ss.GetKeyAsString("code")
	if err != nil {
		return nil, nil, nil, err
	}

	teamIdPtr, err := ss.GetKeyAsString("teamId")
	if err != nil {
		return nil, nil, nil, err
	}

	userIdPtr, err := ss.GetKeyAsString("userId")
	if err != nil {
		return nil, nil, nil, err
	}

	return gameCodePtr, teamIdPtr, userIdPtr, nil
}

// SetStandardKeys returns a set of standard session attributes that identify a user in a game.
// StandardKeys include gameCode, teamId, and userId.
func (ss *Service) SetStandardKeys(gameCode *string, teamId *string, userId *string) error {
	if gameCode == nil {
		return errors.New("gameCode id is required")
	}

	if userId == nil {
		return errors.New("userId id is required")
	}

	ss.SetKey("code", *gameCode)
	ss.SetKey("userId", *userId)

	if teamId != nil {
		ss.SetKey("teamId", *teamId)
	}

	return nil
}

// GetKeyAsString gets a session key and returns its value as a string.
func (ss *Service) GetKeyAsString(key string) (*string, error) {
	keyObject, err := ss.GetKey(key)
	if err != nil {
		return nil, err
	}

	keyValue, ok := keyObject.(string)
	if !ok {
		return nil, fmt.Errorf("%v value is not a string", key)
	}

	return &keyValue, nil
}

// GetKey gets a session key and returns its value as a string.
func (ss *Service) GetKey(key string) (any, error) {
	keyObject, ok := ss.s.Get(key)
	if !ok {
		return nil, fmt.Errorf("no %v in session", key)
	}

	return keyObject, nil
}

// SetKey sets a session key.
func (ss *Service) SetKey(key string, data string) {
	ss.s.Set(key, data)
}

func (ss *Service) UnsetKey(key string) {
	ss.s.UnSet(key)
}

func (ss *Service) Get(gameCode *string, teamId *string, userId *string) (*melody.Session, error) {
	if gameCode != nil && teamId != nil && userId != nil {
		return ss.getSessionForPlayer(*gameCode, *teamId, *userId)
	}

	return nil, errors.New("invalid gameCode, teamId or userId")
}

// Find returns either a session for a specific player, sessions for all players in a given team, or sessions for
// all players in a game, depending on inputs. teamId and playerId can be passed in as nil, but a gameCode is required.
func (ss *Service) Find(gameCode *string, teamId *string, userId *string) ([]*melody.Session, error) {
	if gameCode != nil && teamId != nil && userId != nil {
		retrievedSession, err := ss.getSessionForPlayer(*gameCode, *teamId, *userId)
		return []*melody.Session{retrievedSession}, err
	}

	if gameCode != nil && teamId != nil {
		return ss.getSessionsForTeam(*gameCode, *teamId)
	}

	if gameCode != nil {
		return ss.getSessionsForGame(*gameCode)
	}

	return nil, errors.New("invalid gameCode, teamId or userId")
}

func (ss *Service) getSessionsForGame(gameCode string) ([]*melody.Session, error) {
	allSessions, err := ss.m.Sessions()
	if err != nil {
		return nil, err
	}

	var matchedSessions []*melody.Session

	for _, thisSession := range allSessions {
		checkgameCode, _, _, err := ss.GetStandardKeys()
		if err == nil && *checkgameCode == gameCode {
			matchedSessions = append(matchedSessions, thisSession)
		}
	}

	if len(matchedSessions) == 0 {
		return nil, fmt.Errorf("no sessions were found for game %v", gameCode)
	}

	return matchedSessions, nil
}

func (ss *Service) getSessionsForTeam(gameCode string, teamId string) ([]*melody.Session, error) {
	allSessions, err := ss.m.Sessions()
	if err != nil {
		return nil, err
	}

	var matchedSessions []*melody.Session

	for _, thisSession := range allSessions {
		checkgameCode, checkTeamId, _, err := ss.GetStandardKeys()
		if err == nil && *checkgameCode == gameCode && *checkTeamId == teamId {
			matchedSessions = append(matchedSessions, thisSession)
		}
	}

	if len(matchedSessions) == 0 {
		return nil, fmt.Errorf("no sessions were found for game %v and team %v", gameCode, teamId)
	}

	return matchedSessions, nil
}

func (ss *Service) getSessionForPlayer(
	gameCode string,
	teamId string,
	userId string,
) (*melody.Session, error) {
	allSessions, err := ss.m.Sessions()
	if err != nil {
		return nil, err
	}

	for _, thisSession := range allSessions {
		checkGameCode, checkTeamId, checkUserId, err := ss.GetStandardKeys()
		if err == nil && *checkGameCode == gameCode && *checkTeamId == teamId && *checkUserId == userId {
			return thisSession, nil
		}
	}

	return nil, fmt.Errorf("no sessions were found for userId %v in game %v and team %v", userId, gameCode, teamId)
}
