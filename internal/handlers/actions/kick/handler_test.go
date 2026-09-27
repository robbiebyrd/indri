package kick_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/robbiebyrd/indri/internal/handlers/actions/kick"
	"github.com/robbiebyrd/indri/internal/injector"
	"github.com/robbiebyrd/indri/internal/models"
	gameRepo "github.com/robbiebyrd/indri/internal/repo/game"
	scriptRepo "github.com/robbiebyrd/indri/internal/repo/script"
	sessionRepo "github.com/robbiebyrd/indri/internal/repo/session"
	userRepo "github.com/robbiebyrd/indri/internal/repo/user"
	"github.com/robbiebyrd/indri/internal/services/broadcast"
	"github.com/robbiebyrd/indri/internal/services/events"
	gameService "github.com/robbiebyrd/indri/internal/services/game"
	"github.com/robbiebyrd/indri/internal/services/lock"
	sessionService "github.com/robbiebyrd/indri/internal/services/session"
	"github.com/robbiebyrd/indri/internal/transport"
)

type conn struct {
	mu      sync.Mutex
	keys    map[string]any
	written []string
	closed  bool
}

func newConn(sessionID string) *conn { return &conn{keys: map[string]any{"sessionId": sessionID}} }

func (c *conn) Get(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.keys[key]
	return v, ok
}
func (c *conn) Set(key string, v any) { c.mu.Lock(); c.keys[key] = v; c.mu.Unlock() }
func (c *conn) UnSet(key string)      { c.mu.Lock(); delete(c.keys, key); c.mu.Unlock() }
func (c *conn) Write(msg []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.written = append(c.written, string(msg))
	return nil
}
func (c *conn) WriteBinary(msg []byte) error { return c.Write(msg) }
func (c *conn) Close() error                 { c.mu.Lock(); c.closed = true; c.mu.Unlock(); return nil }
func (c *conn) IsClosed() bool               { c.mu.Lock(); defer c.mu.Unlock(); return c.closed }

type instance struct{ conns []transport.Conn }

func (i *instance) Handle(transport.Handlers)                               {}
func (i *instance) Register(*http.ServeMux)                                 {}
func (i *instance) Broadcast([]byte) error                                  { return nil }
func (i *instance) BroadcastFilter([]byte, func(transport.Conn) bool) error { return nil }
func (i *instance) Conns() ([]transport.Conn, error)                        { return i.conns, nil }
func (i *instance) Close() error                                            { return nil }
func (i *instance) IsClosed() bool                                          { return false }

// relay is an in-test cluster bus: every delivery reaches every subscriber.
type relay struct {
	mu   sync.Mutex
	subs []chan events.Delivery
}

func (r *relay) Publish(_ context.Context, d events.Delivery) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.subs {
		s <- d
	}
	return nil
}

func (r *relay) Subscribe(context.Context) (<-chan events.Delivery, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ch := make(chan events.Delivery, 16)
	r.subs = append(r.subs, ch)
	return ch, nil
}

// must unwraps a setup call's result; a setup failure panics, failing the test.
func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// A host on one instance kicks a player connected to another: the player must
// be disconnected there, not only removed from the game.
func TestKick_DisconnectsATargetConnectedToAnotherInstance(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	scriptPath := filepath.Join(t.TempDir(), "script.json")
	if err := os.WriteFile(scriptPath, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	games := must(gameRepo.NewMemoryStore(ctx, lock.NewInProcess(), events.NewInProcess()))
	sessions := must(sessionRepo.NewMemoryStore(ctx))
	users := must(userRepo.NewMemoryStore(ctx))
	bus := &relay{}

	g := must(games.New("KICK", &models.Script{
		Config: models.Config{MaxPlayersPerTeam: 2},
		Teams:  map[string]models.Team{"red": {Name: "Red"}},
	}, false))
	hostSlot := must(games.AssignSlot(g.ID, "red", "host", "Host"))
	targetSlot := must(games.AssignSlot(g.ID, "red", "target", "Target"))
	host := must(sessions.New(models.CreateSession{UserID: "host", GameID: g.ID, TeamID: "red", SlotID: hostSlot}))
	target := must(sessions.New(models.CreateSession{UserID: "target", GameID: g.ID, TeamID: "red", SlotID: targetSlot}))

	// Instance B holds the target's connection and relays deliveries to it.
	targetConn := newConn(target.ID)
	b := must(broadcast.NewService(ctx, &instance{conns: []transport.Conn{targetConn}}, users, sessions, bus))
	ready := make(chan struct{})
	go func() { _ = b.RelayDeliveries(ctx, ready) }()
	<-ready

	// Instance A holds the host's connection and runs the kick.
	hostConn := newConn(host.ID)
	aTransport := &instance{conns: []transport.Conn{hostConn}}
	i := &injector.Injector{
		ClientsInjector: &injector.ClientsInjector{Transport: aTransport, Deliveries: bus},
		ReposInjector:   &injector.ReposInjector{GameRepo: games, SessionRepo: sessions, UserRepo: users},
		ServicesInjector: &injector.ServicesInjector{
			GameService:      must(gameService.NewService(games, must(scriptRepo.NewStore(scriptPath)))),
			SessionService:   sessionService.NewService(sessions),
			BroadcastService: must(broadcast.NewService(ctx, aTransport, users, sessions, bus)),
		},
		GlobalContext: ctx,
	}

	if err := kick.New(i).Handle(hostConn, map[string]interface{}{"code": "KICK", "slotId": targetSlot}); err != nil {
		t.Fatalf("kick: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for !targetConn.IsClosed() {
		if time.Now().After(deadline) {
			t.Fatal("the kicked player's connection on the other instance was never closed")
		}
		time.Sleep(5 * time.Millisecond)
	}

	after := must(games.Get(g.ID))
	if after.Players[targetSlot].UserID != "" {
		t.Fatalf("target still holds slot %s: %+v", targetSlot, after.Players[targetSlot])
	}
	if hostConn.IsClosed() {
		t.Fatal("the host was disconnected")
	}
}
