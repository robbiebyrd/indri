package game

import (
	"fmt"
)

// HasPlayerOnTeam determines if a given slotId is assigned to a given team.
func (s *Store) HasPlayerOnTeam(id string, teamId string, slotId string) bool {
	g, err := s.Get(id)
	if err != nil {
		return false
	}

	team, ok := g.Teams[teamId]
	if !ok {
		return false
	}

	for _, pId := range team.PlayerIDs {
		if pId == slotId {
			return true
		}
	}

	return false
}

// PlayerOnWhichTeam returns the team a slot is assigned to.
func (s *Store) PlayerOnWhichTeam(id string, slotId string) (*string, error) {
	g, err := s.Get(id)
	if err != nil {
		return nil, fmt.Errorf("failed retrieving game with id %v", id)
	}

	for tId, team := range g.Teams {
		for _, pId := range team.PlayerIDs {
			if pId == slotId {
				return &tId, nil
			}
		}
	}

	return nil, fmt.Errorf("slot %v not found in any team in game %v", slotId, id)
}
