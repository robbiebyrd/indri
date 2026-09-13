package game

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/services/events"
	"github.com/robbiebyrd/indri/internal/services/lock"
)

// MemoryStore is a game store that keeps its documents in a map instead of in
// MongoDB. It is production code, not a test helper, for two reasons: the
// script test harness runs a game's fixtures without a database, and the tests
// of every rule built on Mutate must run on a CI runner that has no MongoDB
// service — otherwise they skip themselves and report green without ever
// having run.
//
// It is a real store, not a stub. Every method a caller sees comes from the
// embedded core and is the same code the MongoDB store runs: the same lock,
// the same version fence, the same retry budget, the same change deltas. Only
// the storage below core differs.
//
// What it cannot do is prove MongoDB's own guarantees. Its version fence is a
// Go comparison under a mutex, not a conditional UpdateOne, so it says nothing
// about whether Mongo's CAS filter behaves as expected. The MongoDB-backed
// tests remain the layer that proves that, and they run wherever a database is
// reachable.
type MemoryStore struct {
	core
}

// NewMemoryStore builds an in-memory game store. A nil publisher is allowed
// and simply drops the deltas, matching the MongoDB store.
func NewMemoryStore(ctx context.Context, locks lock.Manager, publisher events.Publisher) *MemoryStore {
	if locks == nil {
		locks = lock.NewInProcess()
	}

	store := &memoryDocs{games: map[string]*models.Game{}}

	return &MemoryStore{
		core: core{
			ctx:       ctx,
			docs:      store,
			locks:     locks,
			publisher: publisher,
		},
	}
}

// memoryDocs is core's persistence port over a map guarded by a mutex.
//
// Every document crossing the boundary is copied through BSON, exactly as a
// round trip to MongoDB would be. That is not ceremony: handing out the stored
// pointer would let an apply function mutate the stored game before the
// version fence ever ran, and a lost-update test over an aliased map proves
// nothing at all.
type memoryDocs struct {
	mu    sync.Mutex
	games map[string]*models.Game
}

func (m *memoryDocs) insert(_ context.Context, create models.CreateGame) (string, error) {
	g, err := gameFromCreate(create)
	if err != nil {
		return "", err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// The MongoDB store has a unique index on code; a duplicate must fail the
	// same way here or a test would not notice the index going missing.
	for _, stored := range m.games {
		if stored.Code == create.Code {
			return "", fmt.Errorf("game with code %s already exists", create.Code)
		}
	}

	g.ID = bson.NewObjectID()
	m.games[g.ID.Hex()] = g

	return g.ID.Hex(), nil
}

func (m *memoryDocs) load(_ context.Context, id string) (*models.Game, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.get(id)
}

// get returns a copy of the stored game. The caller must hold the mutex.
func (m *memoryDocs) get(id string) (*models.Game, error) {
	stored, ok := m.games[id]
	if !ok {
		return nil, fmt.Errorf("fetching game %q: %w", id, errNotFound)
	}

	return copyGame(stored)
}

func (m *memoryDocs) findByCode(_ context.Context, code string) (*models.Game, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, stored := range m.games {
		if stored.Code == code {
			return copyGame(stored)
		}
	}

	return nil, fmt.Errorf("fetching game with code %q: %w", code, errNotFound)
}

func (m *memoryDocs) findOpen(_ context.Context, limit int) ([]*models.Game, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	ids := make([]string, 0, len(m.games))

	for id, stored := range m.games {
		if !stored.Private {
			ids = append(ids, id)
		}
	}

	// Map iteration order is random; sort so a caller sees a stable page.
	sort.Strings(ids)

	open := make([]*models.Game, 0, len(ids))

	for _, id := range ids {
		if limit > 0 && len(open) == limit {
			break
		}

		g, err := m.get(id)
		if err != nil {
			return nil, err
		}

		open = append(open, g)
	}

	return open, nil
}

func (m *memoryDocs) existsByCode(_ context.Context, code string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, stored := range m.games {
		if stored.Code == code {
			return true, nil
		}
	}

	return false, nil
}

// saveVersioned is the in-memory version fence: the write lands only while the
// stored version is still the one the caller loaded.
func (m *memoryDocs) saveVersioned(
	_ context.Context,
	id string,
	g *models.Game,
	expectedVersion int64,
) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	stored, ok := m.games[id]
	if !ok {
		return false, fmt.Errorf("saving game %q: %w", id, errNotFound)
	}

	if stored.Version != expectedVersion {
		return false, nil
	}

	g.UpdatedAt = time.Now()
	g.Version = expectedVersion + 1

	saved, err := copyGame(g)
	if err != nil {
		return false, err
	}

	// The _id is immutable in the MongoDB store, which never $sets it.
	saved.ID = stored.ID
	m.games[id] = saved

	return true, nil
}

func (m *memoryDocs) replace(_ context.Context, id string, update *models.UpdateGame) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	stored, ok := m.games[id]
	if !ok {
		return fmt.Errorf("error updating game: game with id %v does not exists", id)
	}

	applyUpdate(stored, update)

	return nil
}

func (m *memoryDocs) setField(_ context.Context, id string, key string, value interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	stored, ok := m.games[id]
	if !ok {
		return fmt.Errorf("error updating game field: game with id %v does not exists", id)
	}

	if err := setPath(stored, key, value); err != nil {
		return err
	}

	stored.UpdatedAt = time.Now()
	stored.Version++

	return nil
}

func (m *memoryDocs) unsetField(_ context.Context, id string, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	stored, ok := m.games[id]
	if !ok {
		return fmt.Errorf("field %v does not exists", key)
	}

	if err := unsetPath(stored, key); err != nil {
		return err
	}

	stored.UpdatedAt = time.Now()
	stored.Version++

	return nil
}

func (m *memoryDocs) setPlayerConnected(_ context.Context, id string, userId string, connected bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	stored, ok := m.games[id]
	if !ok {
		return fmt.Errorf("no player %v found in game %v", userId, id)
	}

	player, ok := stored.Players[userId]
	if !ok {
		// Mirrors the MongoDB filter's $exists guard: a player who has been
		// removed must not be recreated by a connect.
		return fmt.Errorf("no player %v found in game %v", userId, id)
	}

	player.Connected = connected
	stored.Players[userId] = player
	stored.UpdatedAt = time.Now()
	stored.Version++

	return nil
}

// errNotFound stands in for the driver's ErrNoDocuments. Callers of the store
// only ever check for a non-nil error, so the distinction does not travel any
// further than this package.
var errNotFound = fmt.Errorf("no documents in result")

// copyGame deep-copies a game through BSON, the same encoding MongoDB stores
// it in, so the in-memory store hands out documents that have survived the
// same round trip a real read does.
func copyGame(g *models.Game) (*models.Game, error) {
	raw, err := bson.Marshal(g)
	if err != nil {
		return nil, fmt.Errorf("copying game: %w", err)
	}

	var copied models.Game
	if err := bson.Unmarshal(raw, &copied); err != nil {
		return nil, fmt.Errorf("copying game: %w", err)
	}

	// bson.Marshal drops an omitempty zero ObjectID, so carry it over.
	copied.ID = g.ID

	return &copied, nil
}

// gameFromCreate turns the insert document into the stored game, through the
// same BSON encoding MongoDB would use.
func gameFromCreate(create models.CreateGame) (*models.Game, error) {
	raw, err := bson.Marshal(create)
	if err != nil {
		return nil, fmt.Errorf("creating game: %w", err)
	}

	var g models.Game
	if err := bson.Unmarshal(raw, &g); err != nil {
		return nil, fmt.Errorf("creating game: %w", err)
	}

	return &g, nil
}

// applyUpdate is the in-memory equivalent of a $set of the non-nil fields of
// an UpdateGame.
func applyUpdate(g *models.Game, update *models.UpdateGame) {
	if update.Teams != nil {
		g.Teams = *update.Teams
	}

	if update.Players != nil {
		g.Players = *update.Players
	}

	if update.Stage != nil {
		g.Stage = *update.Stage
	}

	if update.PublicData != nil {
		g.PublicData = update.PublicData
	}

	if update.PrivateData != nil {
		g.PrivateData = update.PrivateData
	}

	if update.PlayerData != nil {
		g.PlayerData = update.PlayerData
	}

	g.Private = update.Private
	g.UpdatedAt = update.UpdatedAt
}

// setPath writes value at a dotted BSON path, creating intermediate maps as
// MongoDB's $set does. It works on the document's BSON view and decodes the
// result back, so the path vocabulary is exactly the one the Mongo store uses.
func setPath(g *models.Game, path string, value interface{}) error {
	return editPath(g, path, func(parent bson.M, key string) error {
		parent[key] = value

		return nil
	})
}

// unsetPath removes a dotted BSON path, the in-memory $unset.
func unsetPath(g *models.Game, path string) error {
	return editPath(g, path, func(parent bson.M, key string) error {
		if _, ok := parent[key]; !ok {
			return fmt.Errorf("field %v does not exists", path)
		}

		delete(parent, key)

		return nil
	})
}

// editPath decodes the game to a BSON map, walks path to its parent, applies
// edit and decodes the result back onto g.
func editPath(g *models.Game, path string, edit func(parent bson.M, key string) error) error {
	raw, err := bson.Marshal(g)
	if err != nil {
		return fmt.Errorf("editing %q: %w", path, err)
	}

	doc, err := decodeToM(raw)
	if err != nil {
		return fmt.Errorf("editing %q: %w", path, err)
	}

	parent := doc
	segments := splitPath(path)

	for i, segment := range segments[:len(segments)-1] {
		existing, present := parent[segment]
		if !present {
			next := bson.M{}
			parent[segment] = next
			parent = next

			continue
		}

		// MongoDB's $set creates a missing intermediate document, but refuses
		// to descend into one that exists and is not a document — including an
		// explicit null. Diverging here would let a test pass against this
		// store and fail against the real one.
		next, isDocument := existing.(bson.M)
		if !isDocument {
			return fmt.Errorf("cannot create field %q in element {%s: %v}",
				segments[i+1], segment, existing)
		}

		parent = next
	}

	if err := edit(parent, segments[len(segments)-1]); err != nil {
		return err
	}

	edited, err := bson.Marshal(doc)
	if err != nil {
		return fmt.Errorf("editing %q: %w", path, err)
	}

	id := g.ID

	*g = models.Game{}
	if err := bson.Unmarshal(edited, g); err != nil {
		return fmt.Errorf("editing %q: %w", path, err)
	}

	g.ID = id

	return nil
}

// decodeToM decodes a BSON document into nested maps. The driver's default is
// an ordered bson.D for every embedded document, which a path walk cannot
// index; DefaultDocumentM asks for maps all the way down.
func decodeToM(raw []byte) (bson.M, error) {
	decoder := bson.NewDecoder(bson.NewDocumentReader(bytes.NewReader(raw)))
	decoder.DefaultDocumentM()

	var doc bson.M
	if err := decoder.Decode(&doc); err != nil {
		return nil, err
	}

	return doc, nil
}

// splitPath splits a dotted BSON path into its segments.
func splitPath(path string) []string {
	segments := []string{}
	current := ""

	for _, r := range path {
		if r == '.' {
			segments = append(segments, current)
			current = ""

			continue
		}

		current += string(r)
	}

	return append(segments, current)
}
