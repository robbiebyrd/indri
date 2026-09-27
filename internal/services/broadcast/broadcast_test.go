package broadcast_test

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/robbiebyrd/indri/internal/models"
	sessionRepo "github.com/robbiebyrd/indri/internal/repo/session"
	userRepo "github.com/robbiebyrd/indri/internal/repo/user"
	"github.com/robbiebyrd/indri/internal/services/broadcast"
	"github.com/robbiebyrd/indri/internal/services/events"
	"github.com/robbiebyrd/indri/internal/transport"
)

// conn is a connection that records what it is sent. It asks for debug (JSON
// text) encoding so tests can read payloads directly.
type conn struct {
	mu      sync.Mutex
	keys    map[string]any
	written []string
	closed  bool
}

func newConn(sessionID string) *conn {
	return &conn{keys: map[string]any{"sessionId": sessionID, "debug": true}}
}

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

func (c *conn) messages() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.written...)
}

// instance is the transport of one server instance: just its local connections.
type instance struct{ conns []transport.Conn }

func (i *instance) Handle(transport.Handlers)                               {}
func (i *instance) Register(*http.ServeMux)                                 {}
func (i *instance) Broadcast([]byte) error                                  { return nil }
func (i *instance) BroadcastFilter([]byte, func(transport.Conn) bool) error { return nil }
func (i *instance) Conns() ([]transport.Conn, error)                        { return i.conns, nil }
func (i *instance) Close() error                                            { return nil }
func (i *instance) IsClosed() bool                                          { return false }

// bus fans every published delivery out to every subscriber, the publisher's
// own instance included, as Redis Pub/Sub does.
type bus struct {
	mu        sync.Mutex
	subs      []chan events.Delivery
	published int
}

func (b *bus) Publish(_ context.Context, d events.Delivery) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.published++
	for _, s := range b.subs {
		s <- d
	}
	return nil
}

func (b *bus) Subscribe(context.Context) (<-chan events.Delivery, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan events.Delivery, 16)
	b.subs = append(b.subs, ch)
	return ch, nil
}

func (b *bus) publishes() int { b.mu.Lock(); defer b.mu.Unlock(); return b.published }

// cluster is a set of instances sharing one session store (the database) and,
// when relay is non-nil, one delivery bus.
type cluster struct {
	sessions sessionRepo.Storer
	users    userRepo.Storer
	relay    *bus
}

func newCluster(t *testing.T, relay *bus) *cluster {
	t.Helper()
	ctx := context.Background()
	sessions, err := sessionRepo.NewMemoryStore(ctx)
	if err != nil {
		t.Fatal(err)
	}
	users, err := userRepo.NewMemoryStore(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return &cluster{sessions: sessions, users: users, relay: relay}
}

// sessionInGame stores a session for userID in gameID and returns its id.
func (c *cluster) sessionInGame(t *testing.T, userID, gameID string) string {
	t.Helper()
	s, err := c.sessions.New(models.CreateSession{UserID: userID, GameID: gameID, TeamID: "red", SlotID: "p0"})
	if err != nil {
		t.Fatal(err)
	}
	return s.ID
}

// start builds one instance's broadcast service holding conns, relaying
// deliveries until the test ends.
func (c *cluster) start(t *testing.T, conns ...transport.Conn) *broadcast.Service {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	var relay events.Bus[events.Delivery]
	if c.relay != nil {
		relay = c.relay
	}

	s, err := broadcast.NewService(ctx, &instance{conns: conns}, c.users, c.sessions, relay)
	if err != nil {
		t.Fatal(err)
	}

	if c.relay != nil {
		ready := make(chan struct{})
		go func() { _ = s.RelayDeliveries(ctx, ready) }()
		<-ready
	}

	return s
}

// eventually waits for cond, failing the test with msg if it never holds.
func eventually(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal(msg)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestBroadcast_SingleInstanceWritesToTheGamesConnections(t *testing.T) {
	c := newCluster(t, nil)
	inGame := newConn(c.sessionInGame(t, "u1", "g1"))
	elsewhere := newConn(c.sessionInGame(t, "u2", "g2"))
	s := c.start(t, inGame, elsewhere)

	game := "g1"
	if err := s.Broadcast(&game, nil, map[string]string{"hello": "g1"}); err != nil {
		t.Fatal(err)
	}

	if got := inGame.messages(); len(got) != 1 || got[0] != `{"hello":"g1"}` {
		t.Fatalf("player in g1 got %v", got)
	}
	if got := elsewhere.messages(); len(got) != 0 {
		t.Fatalf("player in g2 got %v", got)
	}
}

func TestBroadcast_ReachesPlayersConnectedToOtherInstances(t *testing.T) {
	c := newCluster(t, &bus{})
	remote := newConn(c.sessionInGame(t, "u1", "g1"))
	a := c.start(t)
	c.start(t, remote)

	game := "g1"
	if err := a.Broadcast(&game, nil, map[string]string{"hello": "g1"}); err != nil {
		t.Fatal(err)
	}

	eventually(t, func() bool { return len(remote.messages()) == 1 }, "the player on the other instance never got the broadcast")
	if got := remote.messages()[0]; got != `{"hello":"g1"}` {
		t.Fatalf("got %q", got)
	}
}

func TestCloseSessions_ClosesTheConnectionOnWhicheverInstanceHoldsIt(t *testing.T) {
	c := newCluster(t, &bus{})
	session := c.sessionInGame(t, "u1", "g1")
	remote := newConn(session)
	bystander := newConn(c.sessionInGame(t, "u2", "g1"))
	a := c.start(t)
	c.start(t, remote, bystander)

	if err := a.CloseSessions(session); err != nil {
		t.Fatal(err)
	}

	eventually(t, remote.IsClosed, "the kicked player's connection on the other instance was never closed")
	if got := remote.messages(); len(got) != 1 || got[0] != `{"disconnected": true}` {
		t.Fatalf("kicked player got %v, want one disconnected notice", got)
	}
	if bystander.IsClosed() || len(bystander.messages()) != 0 {
		t.Fatal("a player who wasn't kicked was touched")
	}
}

func TestCloseSessions_SingleInstanceClosesDirectly(t *testing.T) {
	c := newCluster(t, nil)
	session := c.sessionInGame(t, "u1", "g1")
	local := newConn(session)
	s := c.start(t, local)

	if err := s.CloseSessions(session); err != nil {
		t.Fatal(err)
	}

	if !local.IsClosed() {
		t.Fatal("connection not closed")
	}
}

// Every instance receives each game change and delivers it to its own
// connections, so that path must not publish again: each instance would
// otherwise re-broadcast every delta to the whole cluster.
func TestBroadcastLocal_OnlyWritesToThisInstance(t *testing.T) {
	relay := &bus{}
	c := newCluster(t, relay)
	local := newConn(c.sessionInGame(t, "u1", "g1"))
	remote := newConn(c.sessionInGame(t, "u2", "g1"))
	a := c.start(t, local)
	c.start(t, remote)

	game := "g1"
	if err := a.BroadcastLocal(&game, map[string]string{"delta": "1"}); err != nil {
		t.Fatal(err)
	}

	if got := local.messages(); len(got) != 1 {
		t.Fatalf("local player got %v", got)
	}
	if relay.publishes() != 0 {
		t.Fatalf("BroadcastLocal published %d deliveries to the cluster", relay.publishes())
	}
	time.Sleep(50 * time.Millisecond)
	if got := remote.messages(); len(got) != 0 {
		t.Fatalf("remote player got %v", got)
	}
}
