package game

import (
	"context"
	"log"
	"time"

	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/services/events"
	"github.com/robbiebyrd/indri/internal/services/lock"
	"github.com/robbiebyrd/indri/internal/services/mutation"
)

// docs is the persistence half of a game store — the handful of operations a
// backend must supply. It is deliberately narrow and free of game vocabulary:
// it loads and stores documents, and knows nothing about hosts, teams or
// deltas.
//
// Everything above it is backend-independent and lives on core: the
// distributed lock, the version fence, the retry loop, the change delta, and
// every game rule built on those. A second backend therefore reimplements
// storage, not the rules. memory.go is that second backend, and it exists so
// the rules can be tested on a machine with no database — the alternative was
// tests that skip themselves in CI and report green without running.
type docs interface {
	load(ctx context.Context, id string) (*models.Game, error)
	insert(ctx context.Context, doc models.CreateGame) (id string, err error)
	findByCode(ctx context.Context, code string) (*models.Game, error)
	findOpen(ctx context.Context, limit int) ([]*models.Game, error)

	// existsByCode reports whether a game with this code is stored. Exists
	// passes what it calls an id; it has always queried the code field.
	existsByCode(ctx context.Context, code string) (bool, error)

	// saveVersioned persists the whole game only while its stored version is
	// still expectedVersion, bumping the version on success, and reports
	// whether the write committed. This is the version fence.
	saveVersioned(
		ctx context.Context,
		id string,
		g *models.Game,
		expectedVersion int64,
	) (committed bool, err error)

	// replace applies a partial update without a version fence.
	replace(ctx context.Context, id string, update *models.UpdateGame) error

	// setField and unsetField are single-field writes that do not depend on
	// prior state. Both bump the version so they stay coherent with the fence.
	setField(ctx context.Context, id string, key string, value interface{}) error
	unsetField(ctx context.Context, id string, key string) error

	// setPlayerConnected flips one player's connected flag, and only while
	// that player still exists, so a concurrent removal cannot recreate a
	// partial player document.
	setPlayerConnected(ctx context.Context, id string, userId string, connected bool) error
}

// core is the backend-independent half of a game store. Both Store and
// MemoryStore embed one, so a rule such as "the first player to join is the
// host", and the obligation to publish a delta for every write, exist once and
// are exercised by the tests of both.
type core struct {
	// ctx is the process-lifetime context handed in at boot. It bounds writes
	// that have no caller to speak of (change-event fan-out) and the
	// convenience helpers below, which take no context of their own yet. A
	// request-scoped deadline must never be derived from it — that is what
	// Mutate's ctx is for.
	ctx       context.Context
	docs      docs
	locks     lock.Manager
	publisher events.Publisher
}

// New creates a new game, given a code.
func (s *core) New(code string, script *models.Script, privateGame bool) (*models.Game, error) {
	id, err := s.docs.insert(s.ctx, newGameDoc(code, script, privateGame))
	if err != nil {
		return nil, err
	}

	return s.Get(id)
}

// Get retrieves game data for a specific game ID.
func (s *core) Get(id string) (*models.Game, error) {
	return s.docs.load(s.ctx, id)
}

// FindByCode retrieves game data by its game code.
func (s *core) FindByCode(gameCode string) (*models.Game, error) {
	return s.docs.findByCode(s.ctx, gameCode)
}

// FindOpen retrieves every game that is not private, up to limit.
func (s *core) FindOpen(limit int) ([]*models.Game, error) {
	return s.docs.findOpen(s.ctx, limit)
}

// GetIDHex returns the game id for a given game code.
func (s *core) GetIDHex(gameCode string) (*string, error) {
	retrievedGame, err := s.FindByCode(gameCode)
	if err != nil {
		return nil, err
	}

	gameId := retrievedGame.ID.Hex()

	return &gameId, nil
}

// Exists checks to see if a game with the given code already exists.
func (s *core) Exists(id string) (bool, error) {
	return s.docs.existsByCode(s.ctx, id)
}

// Update saves game data to the repository.
func (s *core) Update(id string, game *models.UpdateGame) error {
	game.UpdatedAt = time.Now()

	return s.docs.replace(s.ctx, id, game)
}

// UpdateField updates a single field in the game and publishes the delta.
func (s *core) UpdateField(id string, key string, value interface{}) error {
	if err := s.docs.setField(s.ctx, id, key, value); err != nil {
		return err
	}

	s.publish(id, events.OpUpdate, map[string]interface{}{key: value}, nil)

	return nil
}

// DeleteField removes a field from a game and publishes the delta.
func (s *core) DeleteField(id string, key string) error {
	if err := s.docs.unsetField(s.ctx, id, key); err != nil {
		return err
	}

	s.publish(id, events.OpUpdate, nil, []string{key})

	return nil
}

// Mutate applies apply to the game as a conflict-free read-modify-write. The
// coordination (distributed lock + version fence + retry) lives in the
// mutation package, above this store; core only supplies how to load the game
// and how to save it conditionally on its version. A different backend
// (SQLite, in-memory, ...) reuses the same coordinator by implementing just
// those two operations.
//
// ctx is the caller's context, not the store's: it bounds the wait for the
// game lock and every database round trip this mutation makes, so a caller
// whose deadline expires (a scripted handler, a disconnecting client) unwinds
// instead of pinning a goroutine. The change event published after a commit
// deliberately does not use it — see publish.
func (s *core) Mutate(ctx context.Context, id string, apply func(g *models.Game) error) error {
	_, err := s.MutateResult(ctx, id, apply)

	return err
}

// MutateResult is Mutate, reporting whether the write committed.
//
// Mutate returns nil both when apply aborted and when it wrote, which is enough
// for a caller whose only side effect is the write itself. It is not enough for
// a scripted mutation: apply is re-run on every version-fence miss, so a script
// that queues a reply or a broadcast has queued it once per attempt and only the
// committing attempt's effects may be released. That signal is the whole reason
// this variant exists.
func (s *core) MutateResult(
	ctx context.Context,
	id string,
	apply func(g *models.Game) error,
) (bool, error) {
	var before map[string]interface{}

	return mutation.RunResult(
		ctx,
		s.locks,
		"game:"+id,
		func() (*models.Game, int64, error) {
			g, err := s.docs.load(ctx, id)
			if err != nil {
				return nil, 0, err
			}

			// Snapshot the JSON view before apply mutates the game in place,
			// so the committed delta can be diffed against it.
			before, err = events.ToMap(g)
			if err != nil {
				return nil, 0, err
			}

			return g, g.Version, nil
		},
		apply,
		func(g *models.Game, expectedVersion int64) (bool, error) {
			committed, err := s.docs.saveVersioned(ctx, id, g, expectedVersion)
			if err != nil || !committed {
				return committed, err
			}

			s.publishDiff(id, before, g)

			return true, nil
		},
	)
}

// publishDiff computes the dotted-path delta between the pre-mutation snapshot
// and the saved game and publishes it. Errors are logged, not surfaced: the
// write already committed, so a fan-out hiccup must not fail the mutation.
func (s *core) publishDiff(id string, before map[string]interface{}, after *models.Game) {
	if s.publisher == nil {
		return
	}

	afterMap, err := events.ToMap(after)
	if err != nil {
		log.Printf("could not snapshot game %v for change event: %v", id, err)
		return
	}

	updated, removed := events.Diff(before, afterMap)

	s.publish(id, events.OpUpdate, updated, removed)
}

// publish emits a change event for the game, if a publisher is configured.
//
// It uses the store's own context rather than the caller's on purpose: the
// write has already committed, so a cancelled request must not stop the players
// in that game from learning about it. A write that never publishes is
// invisible.
func (s *core) publish(id string, op events.OperationType, updated map[string]interface{}, removed []string) {
	if s.publisher == nil {
		return
	}

	// Strip private data so a broadcast delta never exposes more than a
	// sanitized keyframe would.
	updated, removed = events.SanitizeDelta(updated, removed)

	event := events.ChangeEvent{
		ID:            id,
		OperationType: op,
		Timestamp:     time.Now(),
		Collection:    collectionName,
		UpdatedFields: updated,
		RemovedFields: removed,
	}

	if !event.HasChanges() {
		return
	}

	if err := s.publisher.Publish(s.ctx, event); err != nil {
		log.Printf("could not publish change event for game %v: %v", id, err)
	}
}

// newGameDoc builds the document a brand new game starts from, stamped out of
// the script. Both backends create games through it, so a game created against
// the in-memory store starts life in exactly the shape a real one does.
func newGameDoc(code string, script *models.Script, privateGame bool) models.CreateGame {
	now := time.Now()

	doc := models.CreateGame{
		Code:      code,
		CreatedAt: now,
		UpdatedAt: now,
		Private:   privateGame,
	}

	if script != nil {
		doc.Teams = &script.Teams
		doc.Stage = &script.Stage
		doc.PublicData = script.PublicData
		doc.PrivateData = script.PrivateData
	}

	return doc
}
