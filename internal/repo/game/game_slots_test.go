package game

import (
	"sort"
	"testing"

	"github.com/robbiebyrd/indri/internal/models"
)

func TestPreDeclareSlots(t *testing.T) {
	script := &models.Script{
		Config: models.Config{MaxPlayersPerTeam: 2},
		Teams: map[string]models.Team{
			"Red":  {Name: "Red"},
			"Blue": {Name: "Blue"},
		},
	}
	players, teams := preDeclareSlots(script)

	if len(players) != 4 {
		t.Errorf("expected 4 slots, got %d", len(players))
	}
	for id, p := range players {
		if p.UserID != "" || p.Name != "" || p.Connected {
			t.Errorf("slot %s not empty: %+v", id, p)
		}
	}

	blueIDs := teams["Blue"].PlayerIDs
	sort.Strings(blueIDs)
	if len(blueIDs) != 2 || blueIDs[0] != "p0" || blueIDs[1] != "p1" {
		t.Errorf("Blue team slots wrong: %v", blueIDs)
	}
	redIDs := teams["Red"].PlayerIDs
	sort.Strings(redIDs)
	if len(redIDs) != 2 || redIDs[0] != "p2" || redIDs[1] != "p3" {
		t.Errorf("Red team slots wrong: %v", redIDs)
	}
}
