// Package handlertest runs action handlers against real in-memory stores and
// services, with fake connections and transports standing in for clients.
package handlertest

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"

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

// Conn is a client connection that records what the server sends it.
type Conn struct {
	mu      sync.Mutex
	keys    map[string]any
	written []string
	closed  bool
}

// NewConn is a connection logged in as sessionID.
func NewConn(sessionID string) *Conn {
	return &Conn{keys: map[string]any{"sessionId": sessionID}}
}

func (c *Conn) Get(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.keys[key]
	return v, ok
}
func (c *Conn) Set(key string, v any) { c.mu.Lock(); c.keys[key] = v; c.mu.Unlock() }
func (c *Conn) UnSet(key string)      { c.mu.Lock(); delete(c.keys, key); c.mu.Unlock() }
func (c *Conn) Write(msg []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.written = append(c.written, string(msg))
	return nil
}
func (c *Conn) WriteBinary(msg []byte) error { return c.Write(msg) }
func (c *Conn) Close() error                 { c.mu.Lock(); c.closed = true; c.mu.Unlock(); return nil }
func (c *Conn) IsClosed() bool               { c.mu.Lock(); defer c.mu.Unlock(); return c.closed }

// Messages returns everything written to the connection so far.
func (c *Conn) Messages() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.written...)
}

// Instance is one server instance's transport: just its local connections.
type Instance struct{ conns []transport.Conn }

// NewInstance is a transport holding conns.
func NewInstance(conns ...transport.Conn) *Instance { return &Instance{conns: conns} }

func (i *Instance) Handle(transport.Handlers)                               {}
func (i *Instance) Register(*http.ServeMux)                                 {}
func (i *Instance) Broadcast([]byte) error                                  { return nil }
func (i *Instance) BroadcastFilter([]byte, func(transport.Conn) bool) error { return nil }
func (i *Instance) Conns() ([]transport.Conn, error)                        { return i.conns, nil }
func (i *Instance) Close() error                                            { return nil }
func (i *Instance) IsClosed() bool                                          { return false }

// Relay is a cluster delivery bus: every delivery reaches every subscriber,
// the publisher's own instance included, as Redis Pub/Sub does.
type Relay struct {
	mu   sync.Mutex
	subs []chan events.Delivery
}

func (r *Relay) Publish(_ context.Context, d events.Delivery) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.subs {
		s <- d
	}
	return nil
}

func (r *Relay) Subscribe(context.Context) (<-chan events.Delivery, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ch := make(chan events.Delivery, 16)
	r.subs = append(r.subs, ch)
	return ch, nil
}

// Stores is the shared state of a cluster: its database.
type Stores struct {
	Games    *gameRepo.MemoryStore
	Sessions *sessionRepo.MemoryStore
	Users    *userRepo.MemoryStore
}

// NewStores is an empty in-memory database.
func NewStores(t *testing.T) Stores {
	t.Helper()
	ctx := context.Background()
	return Stores{
		Games:    Must(gameRepo.NewMemoryStore(ctx, lock.NewInProcess(), events.NewInProcess())),
		Sessions: Must(sessionRepo.NewMemoryStore(ctx)),
		Users:    Must(userRepo.NewMemoryStore(ctx)),
	}
}

// Game creates a game with code and one team, "red", of size slots.
func (s Stores) Game(code string, slots int) *models.Game {
	return Must(s.Games.New(code, &models.Script{
		Config: models.Config{MaxPlayersPerTeam: slots},
		Teams:  map[string]models.Team{"red": {Name: "Red"}},
	}, false))
}

// Seat puts userID in a red slot of gameID with a session recording it, and
// returns the slot and session ids.
func (s Stores) Seat(gameID, userID string) (slot, sessionID string) {
	slot = Must(s.Games.AssignSlot(gameID, "red", userID, userID))
	session := Must(s.Sessions.New(models.CreateSession{UserID: userID, GameID: gameID, TeamID: "red", SlotID: slot}))
	return slot, session.ID
}

// Injector builds one instance's injector over the shared stores. relay may
// be nil for a single instance; otherwise the instance relays deliveries
// until the test ends.
func (s Stores) Injector(t *testing.T, tr transport.Transport, relay events.Bus[events.Delivery]) *injector.Injector {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	scriptPath := filepath.Join(t.TempDir(), "script.json")
	if err := os.WriteFile(scriptPath, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	bs := Must(broadcast.NewService(ctx, tr, s.Users, s.Sessions, relay))
	if relay != nil {
		ready := make(chan struct{})
		go func() { _ = bs.RelayDeliveries(ctx, ready) }()
		<-ready
	}

	return &injector.Injector{
		ClientsInjector: &injector.ClientsInjector{Transport: tr, Deliveries: relay},
		ReposInjector:   &injector.ReposInjector{GameRepo: s.Games, SessionRepo: s.Sessions, UserRepo: s.Users},
		ServicesInjector: &injector.ServicesInjector{
			GameService:      Must(gameService.NewService(s.Games, Must(scriptRepo.NewStore(scriptPath)))),
			SessionService:   sessionService.NewService(s.Sessions),
			BroadcastService: bs,
		},
		GlobalContext: ctx,
	}
}

// Must unwraps a setup call's result; a setup failure panics, failing the test.
func Must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
