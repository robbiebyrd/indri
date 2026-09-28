package inquire_test

import (
	"encoding/json"
	"testing"

	"github.com/robbiebyrd/indri/internal/handlers/actions/inquire"
	ht "github.com/robbiebyrd/indri/internal/handlers/handlertest"
	"github.com/robbiebyrd/indri/internal/models"
)

// gameInfo asks for the game with code and returns the one GameInfo reported.
func gameInfo(t *testing.T, stores ht.Stores, code string) inquire.GameInfo {
	t.Helper()

	conn := ht.NewConn("asker")
	conn.Set("debug", true) // replies as JSON rather than MessagePack
	i := stores.Injector(t, ht.NewInstance(conn), nil)
	i.Script = &models.Script{Config: models.Config{MaxTeams: 1, MaxPlayersPerTeam: 2}}

	msg := map[string]interface{}{"inquiryType": "game", "inquiry": "gameInfo", "code": code}
	if err := inquire.New(i).Handle(conn, msg); err != nil {
		t.Fatalf("inquire: %v", err)
	}

	written := conn.Messages()
	if len(written) != 1 {
		t.Fatalf("got %d messages, want 1: %v", len(written), written)
	}

	var resp struct {
		Games []inquire.GameInfo `json:"games"`
	}
	if err := json.Unmarshal([]byte(written[0]), &resp); err != nil {
		t.Fatalf("decoding %s: %v", written[0], err)
	}
	if len(resp.Games) != 1 {
		t.Fatalf("got %d games, want 1: %s", len(resp.Games), written[0])
	}

	return resp.Games[0]
}

// Every team's slots are pre-declared when the game is created, so fullness
// must count the slots players hold, not the slots that exist.
func TestInquire_TeamIsFullOnlyWhenEverySlotIsHeld(t *testing.T) {
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
			stores := ht.NewStores(t)
			g := stores.Game("FULL", 2)
			for _, user := range tc.seated {
				stores.Seat(g.ID, user)
			}

			info := gameInfo(t, stores, "FULL")

			if len(info.Teams) != 1 || info.Teams[0].Full != tc.full {
				t.Fatalf("teams = %+v, want one team with full=%v", info.Teams, tc.full)
			}
			if info.Full != tc.full {
				t.Fatalf("game full = %v, want %v", info.Full, tc.full)
			}
		})
	}
}
