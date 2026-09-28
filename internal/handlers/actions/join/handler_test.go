package join_test

import (
	"testing"

	"github.com/robbiebyrd/indri/internal/handlers/actions/join"
	ht "github.com/robbiebyrd/indri/internal/handlers/handlertest"
	"github.com/robbiebyrd/indri/internal/models"
)

// A join must name the game and the team; a request missing either is
// rejected up front, before any game is looked up or slot assigned.
func TestJoin_RejectsARequestMissingTheGameCodeOrTeam(t *testing.T) {
	stores := ht.NewStores(t)
	g := stores.Game("JOIN", 1)
	session := ht.Must(stores.Sessions.New(models.CreateSession{UserID: "u1"}))

	conn := ht.NewConn(session.ID)
	i := stores.Injector(t, ht.NewInstance(conn), nil)

	cases := map[string]struct {
		msg  map[string]interface{}
		want string
	}{
		"no code": {map[string]interface{}{"teamId": "red"}, "could not parse gameCode from request"},
		"no team": {map[string]interface{}{"code": "JOIN"}, "could not parse teamId from request"},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			err := join.New(i).Handle(conn, c.msg)
			if err == nil || err.Error() != c.want {
				t.Fatalf("err = %v, want %q", err, c.want)
			}

			for slot, p := range ht.Must(stores.Games.Get(g.ID)).Players {
				if p.UserID != "" {
					t.Fatalf("slot %s was assigned to %q", slot, p.UserID)
				}
			}
		})
	}
}
