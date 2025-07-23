package join

import (
	"encoding/json"
	"fmt"
	"github.com/olahol/melody"
	"github.com/robbiebyrd/indri/internal/entrypoints"
	"github.com/robbiebyrd/indri/internal/handlers/utils"
	"github.com/robbiebyrd/indri/internal/services/game"
	"github.com/robbiebyrd/indri/internal/services/session"
	"log"
)

var gs = game.NewService(nil, nil)

// HandleJoin processes a join game request, and adds a player to a game.
func HandleJoin(
	s *melody.Session,
	decodedMsg map[string]interface{},
) error {
	ss := session.NewService(s)
	fmt.Println(decodedMsg)

	gameCode, teamId := utils.ParseGameCodeAndTeamID(decodedMsg)
	if gameCode == nil {
		return fmt.Errorf("game code not provided")
	}

	userId, err := ss.GetKeyAsString("userId")
	if err != nil {
		s.Write([]byte(`{"authenticated": false}`))
		return fmt.Errorf("unable to get userId: %w", err)
	}

	err = ss.SetStandardKeys(gameCode, nil, userId)
	if err != nil {
		return err
	}

	g, err := gs.GetByCode(*gameCode)
	if err != nil {
		return err
	}

	err = gs.ConnectPlayer(g.ID.Hex(), *teamId, *userId)
	if err != nil {
		log.Printf("error adding player %v to game %v: %v\n", *userId, *gameCode, err)
	}

	gs, err := json.Marshal(g)
	if err != nil {
		return err
	}
	s.Write(gs)

	return nil
}

// HandleLeave processes a leave game request and removes a player from a game.
func HandleLeave(
	s *melody.Session,
	_ map[string]interface{},
) error {
	gs := game.NewService(nil, nil)
	ss := session.NewService(s)

	gameCode, _, playerId, err := ss.GetStandardKeys()
	if err != nil {
		return err
	}

	g, err := gs.GetByCode(*gameCode)
	if err != nil {
		return err
	}

	if *gameCode != g.Code {
		return fmt.Errorf("player is in game %v but asking to leave game %v", *gameCode, g.Code)
	}

	err = gs.RemovePlayer(g.ID.Hex(), *playerId)
	if err != nil {
		log.Printf("could not disconnect player %v from game %v: %v\n", playerId, gameCode, err)
	}

	return nil
}

// HandleKick processes a kick request and removes a player from a game if the requesting player is host.
func HandleKick(
	s *melody.Session,
	decodedMsg map[string]interface{},
) error {
	gs := game.NewService(nil, nil)
	ss := session.NewService(s)

	gameCode, teamId, err := utils.RequireGameCodeAndTeamID(decodedMsg)
	if err != nil {
		return err
	}

	userId, ok := decodedMsg["userId"].(string)
	if !ok {
		return fmt.Errorf("userId to kick must be provided")
	}

	actorGameCode, _, actorPlayerId, err := ss.GetStandardKeys()
	if err != nil {
		return err
	}

	if *actorGameCode != *gameCode {
		return fmt.Errorf("attempt to kick %v from game %v because player %v "+
			"isn't in the same game", userId, *gameCode, *actorPlayerId)
	}

	g, err := gs.GetByCode(*gameCode)
	if err != nil {
		return err
	}

	if !g.Players[*actorPlayerId].Host {
		return fmt.Errorf("attempt to kick %v from game %v failed because %v "+
			"isn't the game host", userId, *gameCode, *actorPlayerId)
	}

	userSession, err := ss.Get(gameCode, teamId, &userId)
	if err != nil {
		return err
	}

	err = gs.RemovePlayer(*gameCode, userId)
	if err != nil {
		log.Printf("could not disconnect player %v from game %v: %v\n", userId, gameCode, err)
	}

	entrypoints.DisconnectPlayer(userSession)

	return nil
}
