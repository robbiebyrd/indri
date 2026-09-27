package game

import (
	"fmt"

	"github.com/robbiebyrd/indri/internal/models"
	repoErrors "github.com/robbiebyrd/indri/internal/repo"
)

// containsID reports whether ids contains target.
func containsID(ids []string, target string) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

// filterOutID returns ids without any occurrence of target.
func filterOutID(ids []string, target string) []string {
	out := ids[:0:0]
	for _, id := range ids {
		if id != target {
			out = append(out, id)
		}
	}
	return out
}

func (s *MemoryStore) HasPlayerOnTeam(id string, teamId string, userId string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	g, ok := s.games[id]
	if !ok {
		return false
	}
	team, ok := g.Teams[teamId]
	if !ok {
		return false
	}
	return containsID(team.PlayerIDs, userId)
}

func (s *MemoryStore) AddPlayerToTeam(id string, teamId string, userId string) error {
	return s.Mutate(id, func(g *models.Game) error {
		team, ok := g.Teams[teamId]
		if !ok {
			return fmt.Errorf("team %q: %w", teamId, repoErrors.ErrNotFound)
		}
		if _, ok := g.Players[userId]; !ok {
			return fmt.Errorf("player %q: %w", userId, repoErrors.ErrNotFound)
		}
		if !containsID(team.PlayerIDs, userId) {
			team.PlayerIDs = append(team.PlayerIDs, userId)
			g.Teams[teamId] = team
		}
		return nil
	})
}

func (s *MemoryStore) RemovePlayerFromTeam(id string, userId string) error {
	return s.Mutate(id, func(g *models.Game) error {
		for tid, team := range g.Teams {
			if containsID(team.PlayerIDs, userId) {
				team.PlayerIDs = filterOutID(team.PlayerIDs, userId)
				g.Teams[tid] = team
			}
		}
		return nil
	})
}

func (s *MemoryStore) ChangePlayerTeam(id string, teamId string, userId string) error {
	return s.Mutate(id, func(g *models.Game) error {
		if _, ok := g.Teams[teamId]; !ok {
			return fmt.Errorf("team %q: %w", teamId, repoErrors.ErrNotFound)
		}
		if _, ok := g.Players[userId]; !ok {
			return fmt.Errorf("player %q: %w", userId, repoErrors.ErrNotFound)
		}
		for tid, team := range g.Teams {
			if containsID(team.PlayerIDs, userId) {
				team.PlayerIDs = filterOutID(team.PlayerIDs, userId)
				g.Teams[tid] = team
			}
		}
		// Read the target team after the removal loop so a player already on
		// it isn't re-added to a stale copy that still lists them.
		newTeam := g.Teams[teamId]
		newTeam.PlayerIDs = append(newTeam.PlayerIDs, userId)
		g.Teams[teamId] = newTeam
		return nil
	})
}

func (s *MemoryStore) PlayerOnWhichTeam(id string, userId string) (*string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	g, ok := s.games[id]
	if !ok {
		return nil, fmt.Errorf("id %q: %w", id, repoErrors.ErrNotFound)
	}
	for tid, team := range g.Teams {
		if containsID(team.PlayerIDs, userId) {
			out := tid
			return &out, nil
		}
	}
	return nil, nil
}
