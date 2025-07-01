package join

import (
	"fmt"
	"github.com/olahol/melody"
	"indri/internal/entrypoints"
	"indri/internal/handlers/utils"
	"indri/internal/models"
	"indri/internal/services/game"
	"indri/internal/services/session"
	"log"
)

// HandleJoin processes a join game request, and adds a player to a game.
func HandleJoin(
	s *melody.Session,
	_ *melody.Melody,
	_ *models.Game,
	decodedMsg map[string]interface{},
) error {
	gs := game.NewService()
	ss := session.NewService(s)

	gameId, teamId, err := utils.RequireGameIDAndTeamID(decodedMsg)
	if err != nil {
		return err
	}

	userId, err := ss.GetKeyAsString("userId")
	if err != nil {
		s.Write([]byte(`{"authenticated": false}`))
		return fmt.Errorf("unable to get userId: %w", err)
	}

	err = ss.SetStandardKeys(gameId, teamId, userId)
	if err != nil {
		return err
	}

	var g *models.Game

	if !gs.Exists(gameId) {
		_, err = gs.New(gameId)
		if err != nil {
			return err
		}
	}

	if gs.HasPlayer(gameId, teamId, userId) {
		g, err = gs.ConnectPlayer(gameId, teamId, userId)
	} else {
		g, err = gs.AddPlayer(gameId, teamId, userId)
	}

	if err != nil {
		log.Printf("error adding player %v to game %v on team %v: %v\n", *userId, *gameId, *teamId, err)
	}

	err = gs.Update(g)
	if err != nil {
		return err
	}

	return nil
}

// HandleLeave processes a leave game request, and remove a player from a game.
func HandleLeave(
	s *melody.Session,
	m *melody.Melody,
	g *models.Game,
	_ map[string]interface{},
) error {
	gs := game.NewService()
	ss := session.NewService(s)

	gameId, teamId, playerId, err := ss.GetStandardKeys()
	if err != nil {
		return err
	}

	if playerId == nil || teamId == nil {
		return fmt.Errorf("playerId and teamId of player to kick must be provided")
	}

	if *gameId != g.Code {
		return fmt.Errorf("player is in game %v but asking to leave game %v", *gameId, g.Code)
	}

	g, err = gs.RemovePlayer(gameId, teamId, playerId)
	if err != nil {
		log.Printf("could not disconnect player %v from game %v: %v\n", playerId, gameId, err)
	}

	err = gs.Update(g)
	if err != nil {
		return err
	}

	entrypoints.HandleDisconnect(s, m)

	return nil
}

// HandleKick processes a kick request, and removes a player from a game if the requesting player is host.
func HandleKick(
	s *melody.Session,
	m *melody.Melody,
	g *models.Game,
	decodedMsg map[string]interface{},
) error {
	gs := game.NewService()
	ss := session.NewService(s)

	gameId, teamId, err := utils.RequireGameIDAndTeamID(decodedMsg)
	if err != nil {
		return err
	}

	userId, ok := decodedMsg["userId"].(string)
	if !ok {
		return fmt.Errorf("userId to kick must be provided")
	}

	actorGameId, actorTeamId, actorPlayerId, err := ss.GetStandardKeys()

	if err != nil {
		return err
	}

	if *actorGameId != *gameId {
		return fmt.Errorf("attempt to kick %v from game %v because player %v "+
			"isn't in the same game", userId, *gameId, *actorPlayerId)
	}

	if !g.Teams[*actorTeamId].Players[*actorPlayerId].Host {
		return fmt.Errorf("attempt to kick %v from game %v failed because %v "+
			"isn't the game host", userId, *gameId, *actorPlayerId)
	}

	userSession, err := ss.Get(m, gameId, teamId, &userId)
	if err != nil {
		return err
	}

	g, err = gs.RemovePlayer(gameId, teamId, &userId)
	if err != nil {
		log.Printf("could not disconnect player %v from game %v: %v\n", userId, gameId, err)
	}

	err = gs.Update(g)
	if err != nil {
		return err
	}

	entrypoints.HandleDisconnect(userSession, m)

	return nil
}
