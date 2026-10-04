package injector

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/robbiebyrd/indri/internal/models"
	gameRepo "github.com/robbiebyrd/indri/internal/repo/game"
	sessionRepo "github.com/robbiebyrd/indri/internal/repo/session"
	userRepo "github.com/robbiebyrd/indri/internal/repo/user"
	broadcastService "github.com/robbiebyrd/indri/internal/services/broadcast"
	"github.com/robbiebyrd/indri/internal/services/events"
	gameService "github.com/robbiebyrd/indri/internal/services/game"
	"github.com/robbiebyrd/indri/internal/services/lock"
	"github.com/robbiebyrd/indri/internal/transport"
)

// registryTransport is the shared connection registry promoted to a transport,
// standing in for one with connections and no wire protocol.
type registryTransport struct{ *transport.Registry }

func (registryTransport) Handle(transport.Handlers) {}
func (registryTransport) Register(*http.ServeMux)   {}

// keyframesFixture is the production game and broadcast services over
// in-memory stores, with a game holding a secret, and a session in it whose
// connection is open.
func keyframesFixture(t *testing.T) (keyframeSender, *transport.BufferedConn[[]byte], string) {
	t.Helper()

	ctx := context.Background()
	games := gameRepo.NewMemoryStore(ctx, lock.NewInProcess(), events.NewInProcess())
	sessions := sessionRepo.NewMemoryStore()
	tr := registryTransport{&transport.Registry{}}

	gs, err := gameService.NewService(games, &models.Script{PrivateData: map[string]interface{}{"answer": "rosebud"}})
	if err != nil {
		t.Fatalf("game service: %v", err)
	}

	bs, err := broadcastService.NewService(ctx, tr, userRepo.NewMemoryStore(), sessions)
	if err != nil {
		t.Fatalf("broadcast service: %v", err)
	}

	g, err := gs.New("KEYFRAMES", false)
	if err != nil {
		t.Fatalf("creating the game: %v", err)
	}

	s, err := sessions.New(models.CreateSession{UserID: "u1", GameID: g.ID})
	if err != nil {
		t.Fatalf("creating the session: %v", err)
	}

	conn := transport.NewBufferedConn[[]byte](s.ID, 4)
	tr.AddSink(conn)

	return keyframeSender{games: gs, broadcast: bs}, conn, g.ID
}

func TestKeyframeSender_KeyframeIsTheSanitizedGame(t *testing.T) {
	k, _, gameID := keyframesFixture(t)

	frame, err := k.Keyframe(gameID)
	if err != nil {
		t.Fatalf("Keyframe: %v", err)
	}

	if !strings.Contains(string(frame), `"code":"KEYFRAMES"`) || strings.Contains(string(frame), "rosebud") {
		t.Fatalf("Keyframe = %s, want the game with its private data stripped", frame)
	}
}

func TestKeyframeSender_BroadcastReachesThePlayersInTheGame(t *testing.T) {
	k, conn, gameID := keyframesFixture(t)

	if err := k.BroadcastKeyframe(gameID); err != nil {
		t.Fatalf("BroadcastKeyframe: %v", err)
	}

	select {
	case frame := <-conn.Events():
		if !strings.Contains(string(frame), `"code":"KEYFRAMES"`) || strings.Contains(string(frame), "rosebud") {
			t.Fatalf("broadcast = %s, want the sanitized game", frame)
		}
	case <-time.After(time.Second):
		t.Fatal("the player in the game received no keyframe")
	}
}
