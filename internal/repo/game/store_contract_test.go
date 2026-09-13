package game

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/robbiebyrd/indri/internal/clients/mongodb"
	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/services/events"
	"github.com/robbiebyrd/indri/internal/services/lock"
	"github.com/robbiebyrd/indri/internal/services/mutation"
)

// The rules in this package — who becomes host, what a mutation publishes,
// what happens when two writers race — belong to the store, not to MongoDB.
// Every test below is therefore written once and run against both backends:
// the in-memory store, which always runs, and the MongoDB store, which runs
// wherever a database is reachable.
//
// That split is deliberate. Before it, every test here needed MongoDB and
// skipped itself on a CI runner that has none, so the whole file reported
// green without executing a single assertion. The in-memory backend closes
// that hole; it does not replace the MongoDB one, which is still the only
// thing that proves the version fence survives a real conditional UpdateOne.

// backend is one store under test, together with the publisher that recorded
// what it broadcast.
type backend struct {
	store  Storer
	events *recordingPublisher
}

// forEachBackend runs test against every available backend. The in-memory one
// always runs; the MongoDB one skips when no database is reachable, which is
// the only skip this package is allowed to have.
func forEachBackend(t *testing.T, test func(t *testing.T, b backend)) {
	t.Helper()

	t.Run("memory", func(t *testing.T) {
		publisher := &recordingPublisher{}
		test(t, backend{
			store:  NewMemoryStore(context.Background(), lock.NewInProcess(), publisher),
			events: publisher,
		})
	})

	t.Run("mongodb", func(t *testing.T) {
		publisher := &recordingPublisher{}
		test(t, backend{store: newTestStore(t, publisher), events: publisher})
	})
}

// newTestStore connects to a local MongoDB (a single-node replica set is not
// required for these repo-level tests). It skips when no database is
// reachable; the in-memory backend carries the same assertions in that case.
func newTestStore(t *testing.T, publisher events.Publisher) *Store {
	t.Helper()

	uri := os.Getenv("INDRI_TEST_MONGO_URI")
	if uri == "" {
		uri = "mongodb://localhost:27017/?directConnection=true"
	}

	_ = os.Setenv("INDRI_MONGO_URI", uri)
	_ = os.Setenv("INDRI_MONGO_DATABASE", "indri_test")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	client, err := mongodb.New(ctx)
	if err != nil {
		t.Skipf("skipping: MongoDB not reachable: %v", err)
	}

	store, err := NewStore(context.Background(), client, lock.NewInProcess(), publisher)
	if err != nil {
		t.Skipf("skipping: could not create game store: %v", err)
	}

	return store
}

// recordingPublisher keeps every delta a store broadcast, so a test can assert
// on what players would have seen. A write that never publishes is invisible
// to them, which makes the absence of an event as interesting as its contents.
type recordingPublisher struct {
	mu     sync.Mutex
	events []events.ChangeEvent
}

func (p *recordingPublisher) Publish(_ context.Context, event events.ChangeEvent) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.events = append(p.events, event)

	return nil
}

func (p *recordingPublisher) Subscribe(context.Context) (<-chan events.ChangeEvent, error) {
	return make(chan events.ChangeEvent), nil
}

func (p *recordingPublisher) drain() []events.ChangeEvent {
	p.mu.Lock()
	defer p.mu.Unlock()

	drained := p.events
	p.events = nil

	return drained
}

// updated merges the updated fields of every recorded delta, in order.
func (p *recordingPublisher) updated() map[string]interface{} {
	merged := map[string]interface{}{}

	for _, event := range p.drain() {
		for key, value := range event.UpdatedFields {
			merged[key] = value
		}
	}

	return merged
}

// newGame creates a game with a code unique to this test run, so parallel runs
// and leftover documents from an earlier run cannot collide.
func newGame(t *testing.T, store Storer, script *models.Script) *models.Game {
	t.Helper()

	g, err := store.New(fmt.Sprintf("test-%d", time.Now().UnixNano()), script, false)
	if err != nil {
		t.Fatalf("creating game: %v", err)
	}

	return g
}

// TestAddPlayer_ConcurrentNoLostUpdates verifies that concurrent AddPlayer
// calls against the same game do not lose players — the exact lost-update race
// the optimistic-concurrency version check is meant to prevent — and that the
// host election inside the mutation stays single-valued while it happens.
func TestAddPlayer_ConcurrentNoLostUpdates(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b backend) {
		gameId := newGame(t, b.store, &models.Script{}).ID.Hex()

		const players = 25

		var wg sync.WaitGroup

		errs := make(chan error, players)

		for i := 0; i < players; i++ {
			wg.Add(1)

			go func(n int) {
				defer wg.Done()

				userId := fmt.Sprintf("user-%d", n)
				if err := b.store.AddPlayer(gameId, userId, userId); err != nil {
					errs <- err
				}
			}(i)
		}

		wg.Wait()
		close(errs)

		for err := range errs {
			t.Errorf("concurrent AddPlayer failed: %v", err)
		}

		final, err := b.store.Get(gameId)
		if err != nil {
			t.Fatalf("reloading game: %v", err)
		}

		if len(final.Players) != players {
			t.Fatalf("expected %d players after concurrent adds, got %d (lost updates)", players, len(final.Players))
		}

		hosts := 0

		for _, p := range final.Players {
			if p.Host {
				hosts++
			}
		}

		if hosts != 1 {
			t.Errorf("expected exactly one host, got %d", hosts)
		}
	})
}

// TestMutate_PublishesTheCommittedDelta is the mutate bracket itself: load,
// snapshot, apply, fence, then publish what actually changed. A write that
// never publishes is invisible to every player in the game, so the delta is
// part of the contract, not a side effect.
func TestMutate_PublishesTheCommittedDelta(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b backend) {
		gameId := newGame(t, b.store, &models.Script{}).ID.Hex()

		if err := b.store.AddPlayer(gameId, "alice", "Alice"); err != nil {
			t.Fatalf("adding the first player: %v", err)
		}

		b.events.drain()

		// The second join is the interesting one: the players map already
		// exists, so the delta must be the dotted path of the new player
		// rather than a wholesale replacement of everyone's state.
		if err := b.store.AddPlayer(gameId, "bob", "Bob"); err != nil {
			t.Fatalf("adding the second player: %v", err)
		}

		updated := b.events.updated()

		player, published := updated["players.bob"]
		if !published {
			t.Fatalf("adding a player published no delta for it: %v", updated)
		}

		fields, ok := player.(map[string]interface{})
		if !ok {
			t.Fatalf("player delta is %T, want the whole player object", player)
		}

		if fields["name"] != "Bob" {
			t.Errorf("published player is %v, want Bob", fields)
		}

		// Alice was host first and stays host, so nothing about her changed
		// and the delta must not mention her.
		if _, mentioned := updated["players.alice"]; mentioned {
			t.Errorf("the delta rewrote an unchanged player: %v", updated)
		}

		if fields["host"] != false {
			t.Errorf("the second player joined as host too: %v", fields)
		}
	})
}

// TestMutate_AbortWritesNothingAndPublishesNothing: an apply that decides no
// change is needed must leave the version alone and stay silent, or clients
// would replay empty deltas and the fence would drift.
func TestMutate_AbortWritesNothingAndPublishesNothing(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b backend) {
		gameId := newGame(t, b.store, &models.Script{}).ID.Hex()

		before, err := b.store.Get(gameId)
		if err != nil {
			t.Fatalf("loading game: %v", err)
		}

		b.events.drain()

		err = b.store.Mutate(context.Background(), gameId, func(g *models.Game) error {
			g.PublicData = map[string]interface{}{"round": 1}

			return mutation.ErrAbort
		})
		if err != nil {
			t.Fatalf("Mutate(ErrAbort) = %v, want no error", err)
		}

		if published := b.events.drain(); len(published) != 0 {
			t.Errorf("an aborted mutation published %d events: %v", len(published), published)
		}

		after, err := b.store.Get(gameId)
		if err != nil {
			t.Fatalf("reloading game: %v", err)
		}

		if after.Version != before.Version {
			t.Errorf("an aborted mutation moved the version from %d to %d", before.Version, after.Version)
		}

		if len(after.PublicData) != 0 {
			t.Errorf("an aborted mutation still wrote %v", after.PublicData)
		}
	})
}

// TestMutate_ApplyErrorIsReturnedAndNothingIsWritten: a rejected move must
// surface as an error to the caller and leave no trace in the game.
func TestMutate_ApplyErrorIsReturnedAndNothingIsWritten(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b backend) {
		gameId := newGame(t, b.store, &models.Script{}).ID.Hex()

		before, err := b.store.Get(gameId)
		if err != nil {
			t.Fatalf("loading game: %v", err)
		}

		b.events.drain()

		rejected := errors.New("not your turn")

		err = b.store.Mutate(context.Background(), gameId, func(g *models.Game) error {
			g.PublicData = map[string]interface{}{"round": 1}

			return rejected
		})
		if !errors.Is(err, rejected) {
			t.Fatalf("Mutate = %v, want %v", err, rejected)
		}

		if published := b.events.drain(); len(published) != 0 {
			t.Errorf("a failed mutation published %d events: %v", len(published), published)
		}

		after, err := b.store.Get(gameId)
		if err != nil {
			t.Fatalf("reloading game: %v", err)
		}

		if after.Version != before.Version || len(after.PublicData) != 0 {
			t.Errorf("a failed mutation wrote anyway: version %d -> %d, data %v",
				before.Version, after.Version, after.PublicData)
		}
	})
}

// TestMutate_PrivateDataNeverReachesTheDelta: the keyframe and the delta are
// sanitized in step, so a mutation that writes private state must broadcast
// the public half and nothing else.
func TestMutate_PrivateDataNeverReachesTheDelta(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b backend) {
		gameId := newGame(t, b.store, &models.Script{}).ID.Hex()

		b.events.drain()

		err := b.store.Mutate(context.Background(), gameId, func(g *models.Game) error {
			g.PublicData = map[string]interface{}{"round": 1}
			g.PrivateData = map[string]interface{}{"answer": "rosebud"}

			return nil
		})
		if err != nil {
			t.Fatalf("Mutate: %v", err)
		}

		published := b.events.drain()
		if len(published) == 0 {
			t.Fatal("writing public data published nothing")
		}

		for _, event := range published {
			for key, value := range event.UpdatedFields {
				if strings.Contains(strings.ToLower(key), "privatedata") {
					t.Errorf("delta leaked a private path %q = %v", key, value)
				}

				if fmt.Sprint(value) == "rosebud" || strings.Contains(fmt.Sprint(value), "rosebud") {
					t.Errorf("delta leaked the private value at %q: %v", key, value)
				}
			}
		}
	})
}

// TestSetPlayerAsHost_LeavesExactlyOneHost: the host flag is spread across the
// players map, so it is only single-valued because the whole rewrite happens
// inside one fenced mutation.
func TestSetPlayerAsHost_LeavesExactlyOneHost(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b backend) {
		gameId := newGame(t, b.store, &models.Script{}).ID.Hex()

		for _, name := range []string{"alice", "bob", "carol"} {
			if err := b.store.AddPlayer(gameId, name, name); err != nil {
				t.Fatalf("adding %s: %v", name, err)
			}
		}

		if err := b.store.SetPlayerAsHost(gameId, "carol"); err != nil {
			t.Fatalf("setting host: %v", err)
		}

		g, err := b.store.Get(gameId)
		if err != nil {
			t.Fatalf("reloading game: %v", err)
		}

		hosts := []string{}

		for id, p := range g.Players {
			if p.Host {
				hosts = append(hosts, id)
			}
		}

		if len(hosts) != 1 || hosts[0] != "carol" {
			t.Errorf("hosts are %v, want exactly [carol]", hosts)
		}

		if err := b.store.SetPlayerAsHost(gameId, "nobody"); err == nil {
			t.Error("promoting a player who is not in the game was accepted")
		}
	})
}

// TestConnectPlayer_RefusesAPlayerWhoHasLeft: the connected flag is written
// outside Mutate, conditional on the player still existing, so a reconnect
// racing a kick cannot resurrect a partial player document.
func TestConnectPlayer_RefusesAPlayerWhoHasLeft(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b backend) {
		gameId := newGame(t, b.store, &models.Script{}).ID.Hex()

		if err := b.store.AddPlayer(gameId, "alice", "Alice"); err != nil {
			t.Fatalf("adding player: %v", err)
		}

		if err := b.store.ConnectPlayer(gameId, "alice"); err != nil {
			t.Fatalf("connecting player: %v", err)
		}

		if updated := b.events.updated(); updated["players.alice.connected"] != true {
			t.Errorf("connecting published %v, want players.alice.connected true", updated)
		}

		if err := b.store.RemovePlayer(gameId, "alice"); err != nil {
			t.Fatalf("removing player: %v", err)
		}

		b.events.drain()

		if err := b.store.ConnectPlayer(gameId, "alice"); err == nil {
			t.Error("connecting a player who has left was accepted")
		}

		if published := b.events.drain(); len(published) != 0 {
			t.Errorf("a refused connect still published %v", published)
		}

		g, err := b.store.Get(gameId)
		if err != nil {
			t.Fatalf("reloading game: %v", err)
		}

		if _, resurrected := g.Players["alice"]; resurrected {
			t.Error("a refused connect recreated the player")
		}
	})
}

// TestUpdateField_PublishesTheFieldItWrote covers the single-field write path,
// which bumps the version itself so it stays coherent with Mutate's fence.
func TestUpdateField_PublishesTheFieldItWrote(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b backend) {
		script := &models.Script{PublicData: map[string]interface{}{"round": 1}}
		gameId := newGame(t, b.store, script).ID.Hex()

		before, err := b.store.Get(gameId)
		if err != nil {
			t.Fatalf("loading game: %v", err)
		}

		b.events.drain()

		if err := b.store.UpdateField(gameId, "data.round", 2); err != nil {
			t.Fatalf("updating field: %v", err)
		}

		if updated := b.events.updated(); fmt.Sprint(updated["data.round"]) != "2" {
			t.Errorf("UpdateField published %v, want data.round = 2", updated)
		}

		after, err := b.store.Get(gameId)
		if err != nil {
			t.Fatalf("reloading game: %v", err)
		}

		if after.Version <= before.Version {
			t.Errorf("UpdateField left the version at %d; Mutate's fence would not notice the write",
				after.Version)
		}

		if fmt.Sprint(after.PublicData["round"]) != "2" {
			t.Errorf("stored public data is %v, want round 2", after.PublicData)
		}
	})
}
