package utils

import (
	"strings"
	"testing"

	"github.com/robbiebyrd/indri/internal/models"
)

func ref(s string) *string {
	return &s
}

// hostedGame is a game whose slot p0 is held by host-user, the host, and p1 by
// guest-user.
func hostedGame() *models.Game {
	return &models.Game{
		ID: "game-1",
		Players: map[string]models.Player{
			"p0": {UserID: "host-user", Host: true},
			"p1": {UserID: "guest-user"},
		},
	}
}

func TestRequireHost(t *testing.T) {
	for _, tc := range []struct {
		name    string
		session *models.Session
		wantErr string
	}{
		{
			name:    "the host in their own slot",
			session: &models.Session{UserID: ref("host-user"), GameID: ref("game-1"), SlotID: ref("p0")},
		},
		{
			name:    "no session",
			session: nil,
			wantErr: "must be logged in",
		},
		{
			name:    "a session with no user",
			session: &models.Session{GameID: ref("game-1"), SlotID: ref("p0")},
			wantErr: "no user id",
		},
		{
			name:    "a caller in another game",
			session: &models.Session{UserID: ref("host-user"), GameID: ref("game-2"), SlotID: ref("p0")},
			wantErr: "not in game",
		},
		{
			name:    "a caller with no slot",
			session: &models.Session{UserID: ref("host-user"), GameID: ref("game-1")},
			wantErr: "no slot",
		},
		{
			name:    "a guest",
			session: &models.Session{UserID: ref("guest-user"), GameID: ref("game-1"), SlotID: ref("p1")},
			wantErr: "not the host",
		},
		{
			// A session that still names the host slot after its user lost it
			// must not carry the host's authority to whoever's session that is.
			name:    "a session naming the host slot its user does not hold",
			session: &models.Session{UserID: ref("guest-user"), GameID: ref("game-1"), SlotID: ref("p0")},
			wantErr: "does not hold",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := RequireHost(tc.session, hostedGame())

			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("RequireHost = %v, want nil", err)
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Fatalf("RequireHost = %v, want an error containing %q", err, tc.wantErr)
			}
		})
	}
}
