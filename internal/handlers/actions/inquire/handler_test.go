package inquire

import (
	"context"
	"testing"

	"github.com/robbiebyrd/indri/internal/injector"
	"github.com/robbiebyrd/indri/internal/models"
	gameRepo "github.com/robbiebyrd/indri/internal/repo/game"
	"github.com/robbiebyrd/indri/internal/services/lock"
)

// Every team's slots are pre-declared when the game is created, so fullness
// must count the slots players hold, not the slots that exist.
func TestCreateGameInfo_TeamIsFullOnlyWhenEverySlotIsHeld(t *testing.T) {
	cases := []struct {
		name   string
		seated []string
		full   bool
	}{
		{name: "no players", seated: nil, full: false},
		{name: "one of two slots held", seated: []string{"alice"}, full: false},
		{name: "both slots held", seated: []string{"alice", "bob"}, full: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			script := &models.Script{
				Config: models.Config{MaxTeams: 1, MaxPlayersPerTeam: 2},
				Teams:  map[string]models.Team{"red": {Name: "Red"}},
			}

			store := gameRepo.NewMemoryStore(context.Background(), lock.NewInProcess(), nil)

			g, err := store.New("FULL", script, false)
			if err != nil {
				t.Fatalf("creating game: %v", err)
			}

			for _, user := range tc.seated {
				if _, _, err := store.AssignSlot(g.ID, "red", user, user); err != nil {
					t.Fatalf("seating %s: %v", user, err)
				}
			}

			g, err = store.Get(g.ID)
			if err != nil {
				t.Fatalf("reading game: %v", err)
			}

			info := New(&injector.Injector{Script: script}).createGameInfo(g)

			if len(info.Teams) != 1 || info.Teams[0].Full != tc.full {
				t.Fatalf("teams = %+v, want one team with full=%v", info.Teams, tc.full)
			}

			if info.Full != tc.full {
				t.Fatalf("game full = %v, want %v", info.Full, tc.full)
			}
		})
	}
}
