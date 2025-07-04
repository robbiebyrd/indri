package join

import (
	"fmt"
	"github.com/olahol/melody"
	"github.com/robbiebyrd/indri/internal/entrypoints"
	"github.com/robbiebyrd/indri/internal/handlers/utils"
	"github.com/robbiebyrd/indri/internal/services/game"
	"github.com/robbiebyrd/indri/internal/services/session"
	"log"
)

// HandleJoin processes a join game request, and adds a player to a game.
func HandleJoin(
	s *melody.Session,
	decodedMsg map[string]interface{},
) error {
	gs := game.NewService()
	ss := session.NewService(s)

	gameCode, teamId, err := utils.RequireGameCodeAndTeamID(decodedMsg)
	if err != nil {
		return err
	}

	userId, err := ss.GetKeyAsString("userId")
	if err != nil {
		s.Write([]byte(`{"authenticated": false}`))
		return fmt.Errorf("unable to get userId: %w", err)
	}

	err = ss.SetStandardKeys(gameCode, teamId, userId)
	if err != nil {
		return err
	}

	if !gs.Exists(gameCode) {
		_, err = gs.New(gameCode)
		if err != nil {
			return err
		}
	}

	if gs.HasPlayer(gameCode, teamId, userId) {
		_, err = gs.ConnectPlayer(gameCode, teamId, userId)
	} else {
		_, err = gs.AddPlayer(gameCode, teamId, userId)
	}

	if err != nil {
		log.Printf("error adding player %v to game %v on team %v: %v\n", *userId, *gameCode, *teamId, err)
	}

	err = gs.Update(gameCode, nil)
	if err != nil {
		return err
	}

	return nil
}

// HandleLeave processes a leave game request, and remove a player from a game.
func HandleLeave(
	s *melody.Session,
	_ map[string]interface{},
) error {
	gs := game.NewService()
	ss := session.NewService(s)

	gameCode, teamId, playerId, err := ss.GetStandardKeys()
	if err != nil {
		return err
	}

	if playerId == nil || teamId == nil {
		return fmt.Errorf("playerId and teamId of player to kick must be provided")
	}

	g, err := gs.Get(gameCode)
	if err != nil {
		return err
	}

	if *gameCode != g.Code {
		return fmt.Errorf("player is in game %v but asking to leave game %v", *gameCode, g.Code)
	}

	g, err = gs.RemovePlayer(gameCode, teamId, playerId)
	if err != nil {
		log.Printf("could not disconnect player %v from game %v: %v\n", playerId, gameCode, err)
	}

	err = gs.Update(gameCode, nil)
	if err != nil {
		return err
	}

	entrypoints.HandleDisconnect(s)

	return nil
}

// HandleKick processes a kick request, and removes a player from a game if the requesting player is host.
func HandleKick(
	s *melody.Session,
	decodedMsg map[string]interface{},
) error {
	gs := game.NewService()
	ss := session.NewService(s)

	gameCode, teamId, err := utils.RequireGameCodeAndTeamID(decodedMsg)
	if err != nil {
		return err
	}

	userId, ok := decodedMsg["userId"].(string)
	if !ok {
		return fmt.Errorf("userId to kick must be provided")
	}

	actorGameCode, actorTeamId, actorPlayerId, err := ss.GetStandardKeys()

	if err != nil {
		return err
	}

	if *actorGameCode != *gameCode {
		return fmt.Errorf("attempt to kick %v from game %v because player %v "+
			"isn't in the same game", userId, *gameCode, *actorPlayerId)
	}

	g, err := gs.Get(gameCode)
	if err != nil {
		return err
	}

	if !g.Teams[*actorTeamId].Players[*actorPlayerId].Host {
		return fmt.Errorf("attempt to kick %v from game %v failed because %v "+
			"isn't the game host", userId, *gameCode, *actorPlayerId)
	}

	userSession, err := ss.Get(gameCode, teamId, &userId)
	if err != nil {
		return err
	}

	g, err = gs.RemovePlayer(gameCode, teamId, &userId)
	if err != nil {
		log.Printf("could not disconnect player %v from game %v: %v\n", userId, gameCode, err)
	}

	err = gs.Update(gameCode, nil)
	if err != nil {
		return err
	}

	entrypoints.HandleDisconnect(userSession)

	return nil
}
