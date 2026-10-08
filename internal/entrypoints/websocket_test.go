package entrypoints_test

import (
	"net/http"
	"testing"

	"github.com/robbiebyrd/indri/internal/entrypoints"
	"github.com/robbiebyrd/indri/internal/handlers/handlertest"
	"github.com/robbiebyrd/indri/internal/transport"
)

// fakeTransport is a Registry promoted to a full Transport, holding whatever
// connections a test adds.
type fakeTransport struct{ *transport.Registry }

func (fakeTransport) Handle(transport.Handlers) {}
func (fakeTransport) Register(*http.ServeMux)   {}

// seatedConnection seats alice in a game and returns the fixture, the game id,
// her slot and a closed connection of hers, as the transport hands it over
// when it ends.
func seatedConnection(t *testing.T) (*handlertest.Fixture, string, string, transport.Conn) {
	t.Helper()

	f := handlertest.New(t, handlertest.TwoTeams())
	g := f.Game(t, "ABCD")
	session := f.Seat(t, "alice", g.ID, "red")

	ended := transport.NewBufferedConn[[]byte](session.ID, 4)
	_ = ended.Close()

	return f, g.ID, *session.SlotID, ended
}

// A connection ending is not the player leaving while their session is still
// connected elsewhere. A user's older WebSocket is closed the moment a newer
// one binds their session, and that close must not show them offline to
// everybody while they are playing on the newer one.
func TestHandleDisconnect_KeepsAPlayerOnlineWhileTheirSessionIsConnectedElsewhere(t *testing.T) {
	f, gameID, slotID, ended := seatedConnection(t)

	sessionID, _ := ended.Get(transport.SessionIDKey)

	tr := fakeTransport{&transport.Registry{}}
	tr.AddSink(transport.NewBufferedConn[[]byte](sessionID.(string), 4))

	entrypoints.HandleDisconnect(ended, tr, f.I.GameService, f.I.SessionService)

	if !f.GameByID(t, gameID).Players[slotID].Connected {
		t.Error("the player was marked offline although their session is still connected")
	}
}

// The control: with no other connection, the player is offline.
func TestHandleDisconnect_MarksAPlayerOfflineWithNoOtherConnection(t *testing.T) {
	f, gameID, slotID, ended := seatedConnection(t)

	tr := fakeTransport{&transport.Registry{}}

	entrypoints.HandleDisconnect(ended, tr, f.I.GameService, f.I.SessionService)

	if f.GameByID(t, gameID).Players[slotID].Connected {
		t.Error("the player is still online after their only connection ended")
	}
}
