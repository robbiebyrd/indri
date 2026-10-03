package game

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/repo/repotest"
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

	t.Run("sqlite", func(t *testing.T) {
		publisher := &recordingPublisher{}
		test(t, backend{store: newTestSQLiteStore(t, publisher), events: publisher})
	})

	t.Run("postgres", func(t *testing.T) {
		publisher := &recordingPublisher{}
		test(t, backend{store: newTestPostgresStore(t, publisher), events: publisher})
	})
}

// newTestSQLiteStore builds a store over a private, file-backed database.
func newTestSQLiteStore(t *testing.T, publisher events.Publisher) *SQLiteStore {
	t.Helper()

	return NewSQLiteStore(context.Background(), repotest.SQLite(t), lock.NewInProcess(), publisher)
}

// newTestPostgresStore builds a store in a PostgreSQL schema of the test's own.
func newTestPostgresStore(t *testing.T, publisher events.Publisher) *PostgresStore {
	t.Helper()

	store, err := NewPostgresStore(context.Background(), repotest.Postgres(t), lock.NewInProcess(), publisher)
	if err != nil {
		t.Fatalf("creating the PostgreSQL game store: %v", err)
	}

	return store
}

// newTestStore builds a store over MongoDB, which skips when none is reachable.
func newTestStore(t *testing.T, publisher events.Publisher) *Store {
	t.Helper()

	store, err := NewStore(context.Background(), repotest.Mongo(t), lock.NewInProcess(), publisher)
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

// TestAssignSlot_ConcurrentNoLostUpdates: concurrent joins must each land in
// their own slot — the lost-update race the version fence exists to prevent —
// and the host election inside the mutation must stay single-valued.
func TestAssignSlot_ConcurrentNoLostUpdates(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b backend) {
		const players = 25

		g := newGame(t, b.store, &models.Script{
			Config: models.Config{MaxPlayersPerTeam: players},
			Teams:  map[string]models.Team{"red": {Name: "Red"}},
		})

		var wg sync.WaitGroup

		errs := make(chan error, players)

		for i := 0; i < players; i++ {
			wg.Add(1)

			go func(n int) {
				defer wg.Done()

				userId := fmt.Sprintf("user-%d", n)
				if _, _, err := b.store.AssignSlot(g.ID, "red", userId, userId); err != nil {
					errs <- err
				}
			}(i)
		}

		wg.Wait()
		close(errs)

		for err := range errs {
			t.Errorf("concurrent AssignSlot failed: %v", err)
		}

		final, err := b.store.Get(g.ID)
		if err != nil {
			t.Fatalf("reloading game: %v", err)
		}

		holders, hosts := map[string]bool{}, 0

		for _, p := range final.Players {
			holders[p.UserID] = true

			if p.Host {
				hosts++
			}
		}

		for i := 0; i < players; i++ {
			if !holders[fmt.Sprintf("user-%d", i)] {
				t.Errorf("user-%d has no slot (lost update)", i)
			}
		}

		if hosts != 1 {
			t.Errorf("expected exactly one host, got %d", hosts)
		}
	})
}

// TestAssignSlot_PublishesTheSeat: a join that never publishes is invisible to
// every other player, and it must publish only the slot that changed.
func TestAssignSlot_PublishesTheSeat(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b backend) {
		g := twoSlotGame(t, b.store)
		assign(t, b.store, g.ID, "blue", "alice")

		b.events.drain()

		slot, _, err := b.store.AssignSlot(g.ID, "blue", "bob", "Bob")
		if err != nil {
			t.Fatalf("AssignSlot: %v", err)
		}

		updated := b.events.updated()

		if updated["players."+slot+".userId"] != "bob" || updated["players."+slot+".name"] != "Bob" {
			t.Fatalf("seating bob in %s published %v", slot, updated)
		}

		// Alice joined first and stays host, so nothing about her changed and the
		// delta must not mention her.
		for key := range updated {
			if strings.HasPrefix(key, "players.") && !strings.HasPrefix(key, "players."+slot+".") {
				t.Errorf("the delta wrote %q, which seating bob did not change", key)
			}
		}
	})
}

// TestMutate_AbortWritesNothingAndPublishesNothing: an apply that decides no
// change is needed must leave the version alone and stay silent, or clients
// would replay empty deltas and the fence would drift.
func TestMutate_AbortWritesNothingAndPublishesNothing(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b backend) {
		gameId := newGame(t, b.store, &models.Script{}).ID

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
		gameId := newGame(t, b.store, &models.Script{}).ID

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
		gameId := newGame(t, b.store, &models.Script{}).ID

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
		gameId := twoSlotGame(t, b.store).ID

		assign(t, b.store, gameId, "red", "alice")
		assign(t, b.store, gameId, "red", "bob")
		carol := assign(t, b.store, gameId, "blue", "carol")

		if err := b.store.SetPlayerAsHost(gameId, carol); err != nil {
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

		if len(hosts) != 1 || hosts[0] != carol {
			t.Errorf("hosts are %v, want exactly carol's slot [%s]", hosts, carol)
		}

		if err := b.store.SetPlayerAsHost(gameId, "p99"); err == nil {
			t.Error("promoting a slot the game does not have was accepted")
		}

		// blue's second slot is free: a host nobody holds would leave the game
		// with a host flag set and so stop the next joiner being elected.
		if err := b.store.SetPlayerAsHost(gameId, "p1"); err == nil {
			t.Error("promoting a free slot was accepted")
		}

		if !b.store.PlayerIsHost(gameId, carol) {
			t.Error("a refused promotion still moved the host")
		}
	})
}

// TestConnectPlayer_DoesNotReseatAPlayerWhoHasLeft: a reconnect racing a kick
// must not bring the kicked player back into their old slot.
func TestConnectPlayer_DoesNotReseatAPlayerWhoHasLeft(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b backend) {
		g := twoSlotGame(t, b.store)
		slot := assign(t, b.store, g.ID, "red", "alice")

		if err := b.store.RemovePlayer(g.ID, slot, "alice"); err != nil {
			t.Fatalf("RemovePlayer: %v", err)
		}

		b.events.drain()

		if err := b.store.ConnectPlayer(g.ID, slot, "alice"); err != nil {
			t.Fatalf("a connect for a slot the user no longer holds should be a no-op, got %v", err)
		}

		if published := b.events.drain(); len(published) != 0 {
			t.Errorf("a refused connect still published %v", published)
		}

		if p := player(t, b.store, g.ID, slot); p.UserID != "" || p.Connected {
			t.Errorf("a refused connect reseated the player: %+v", p)
		}
	})
}

// TestUpdateField_PublishesTheFieldItWrote covers the single-field write path,
// which bumps the version itself so it stays coherent with Mutate's fence.
func TestUpdateField_PublishesTheFieldItWrote(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b backend) {
		script := &models.Script{PublicData: map[string]interface{}{"round": 1}}
		gameId := newGame(t, b.store, script).ID

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

// TestUpdateField_PublishesAStructWithoutItsPrivateData is the other half of
// TestMutate_PrivateDataNeverReachesTheDelta, for the write path that does not
// go through Diff. UpdateField hands one Go value to two places — the store
// keeps it, the delta broadcasts it — and internal/services/stage already
// hands it a whole models.Stage, which carries a private store on the stage
// and another on every scene inside it. What is stored must keep them; what
// every player receives must not.
func TestUpdateField_PublishesAStructWithoutItsPrivateData(t *testing.T) {
	const (
		stageSecret = "stage-answer-must-not-leak"
		sceneSecret = "scene-answer-must-not-leak"
		scenePrompt = "the prompt every player may see"
	)

	forEachBackend(t, func(t *testing.T, b backend) {
		gameId := newGame(t, b.store, &models.Script{}).ID

		b.events.drain()

		scenePrivate := map[string]interface{}{"answer": sceneSecret}
		sceneData := map[string]interface{}{"prompt": scenePrompt}
		stage := models.Stage{
			CurrentScene: "board",
			SceneOrder:   []string{"board"},
			Scenes: map[string]models.Scene{
				"board": {PublicData: &sceneData, PrivateData: &scenePrivate},
			},
			PrivateData: map[string]interface{}{"answer": stageSecret},
		}

		// The call stage.Service.LoadFromScript makes, with the script stage
		// this game would have been stamped from.
		if err := b.store.UpdateField(gameId, "stage", stage); err != nil {
			t.Fatalf("updating the stage: %v", err)
		}

		// Asserted on the JSON form because that is what a player receives:
		// the scene stores are pointers, and printing the value would show
		// their addresses rather than the secrets they hold.
		published, err := json.Marshal(b.events.updated())
		if err != nil {
			t.Fatalf("marshalling the published delta: %v", err)
		}

		for _, secret := range []string{stageSecret, sceneSecret} {
			if bytes.Contains(published, []byte(secret)) {
				t.Errorf("the broadcast delta leaked private data: %s", published)
			}
		}

		if !bytes.Contains(published, []byte(scenePrompt)) {
			t.Errorf("the delta lost the public data along with the private: %s", published)
		}

		// Only the published copy may differ. The stored game is the one the
		// game logic reads, so it has to keep everything it was given.
		stored, err := b.store.Get(gameId)
		if err != nil {
			t.Fatalf("reloading game: %v", err)
		}

		if fmt.Sprint(stored.Stage.PrivateData["answer"]) != stageSecret {
			t.Errorf("stage private data was not stored, got %v", stored.Stage.PrivateData)
		}

		scene, ok := stored.Stage.Scenes["board"]
		if !ok {
			t.Fatalf("the scene was not stored at all, got %v", stored.Stage.Scenes)
		}

		if scene.PrivateData == nil || fmt.Sprint((*scene.PrivateData)["answer"]) != sceneSecret {
			t.Errorf("scene private data was not stored, got %v", scene.PrivateData)
		}

		// Sanitizing must copy, not edit: the caller still holds this stage.
		if stage.PrivateData["answer"] != stageSecret {
			t.Errorf("publishing emptied the caller's own stage, got %v", stage.PrivateData)
		}
	})
}

// TestUpdateField_ConcurrentWritesAreNotLost: single-field writes to different
// keys, racing, must all land and each count once on the version. A backend
// that read the game, set one key and wrote the whole document back would put
// a stale copy of the other keys over a concurrent write.
func TestUpdateField_ConcurrentWritesAreNotLost(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b backend) {
		g := newGame(t, b.store, &models.Script{PublicData: map[string]interface{}{"seed": 0}})

		const writers = 20

		var wg sync.WaitGroup

		for i := range writers {
			wg.Add(1)

			go func() {
				defer wg.Done()

				if err := b.store.UpdateField(g.ID, fmt.Sprintf("data.k%d", i), i); err != nil {
					t.Errorf("UpdateField(k%d): %v", i, err)
				}
			}()
		}

		wg.Wait()

		got, err := b.store.Get(g.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}

		for i := range writers {
			if v := fmt.Sprint(got.PublicData[fmt.Sprintf("k%d", i)]); v != fmt.Sprint(i) {
				t.Errorf("data.k%d = %s, want %d (a concurrent write was lost)", i, v, i)
			}
		}

		if want := g.Version + writers; got.Version != want {
			t.Errorf("version = %d, want %d: every write must count once", got.Version, want)
		}
	})
}
