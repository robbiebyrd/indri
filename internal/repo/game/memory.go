package game

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/robbiebyrd/indri/internal/models"
	repoErrors "github.com/robbiebyrd/indri/internal/repo"
	"github.com/robbiebyrd/indri/internal/repo/ids"
	"github.com/robbiebyrd/indri/internal/services/events"
	"github.com/robbiebyrd/indri/internal/services/lock"
	"github.com/robbiebyrd/indri/internal/services/mutation"
)

// MemoryStore is an in-process, non-persistent game.Storer. State lives in a
// map protected by a single RWMutex; the store returns deep copies of games
// to callers so external mutation cannot corrupt stored state. locks and
// publisher are used identically to MongoStore for cross-instance mutation
// serialisation and change fan-out.
type MemoryStore struct {
	ctx       context.Context
	locks     lock.Manager
	publisher events.Publisher

	mu    sync.RWMutex
	games map[string]*models.Game // id -> game
	codes map[string]string       // code -> id (unique)
}

// TODO(memory): reinstate after all Storer methods land in Task 11.
// var _ Storer = (*MemoryStore)(nil)

func NewMemoryStore(ctx context.Context, locks lock.Manager, publisher events.Publisher) (*MemoryStore, error) {
	if locks == nil {
		return nil, errors.New("locks is required")
	}
	if publisher == nil {
		return nil, errors.New("publisher is required")
	}
	return &MemoryStore{
		ctx:       ctx,
		locks:     locks,
		publisher: publisher,
		games:     make(map[string]*models.Game),
		codes:     make(map[string]string),
	}, nil
}

// copyGame returns a defensive deep copy. Maps at each level are re-created
// so callers cannot mutate stored state via the returned pointer. Leaf
// scalars are shared.
func copyGame(g *models.Game) *models.Game {
	if g == nil {
		return nil
	}
	out := *g
	if g.Teams != nil {
		out.Teams = make(map[string]models.Team, len(g.Teams))
		for k, v := range g.Teams {
			t := v
			// PlayerIDs is a slice: re-slice to unshare backing storage.
			if v.PlayerIDs != nil {
				t.PlayerIDs = append([]string(nil), v.PlayerIDs...)
			}
			if v.PublicData != nil {
				t.PublicData = deepCopyMap(v.PublicData)
			}
			if v.PrivateData != nil {
				t.PrivateData = deepCopyMap(v.PrivateData)
			}
			if v.PlayerData != nil {
				t.PlayerData = make(map[string]map[string]interface{}, len(v.PlayerData))
				for pid, pdata := range v.PlayerData {
					if pdata != nil {
						t.PlayerData[pid] = deepCopyMap(pdata)
					}
				}
			}
			out.Teams[k] = t
		}
	}
	if g.Players != nil {
		out.Players = make(map[string]models.Player, len(g.Players))
		for k, v := range g.Players {
			out.Players[k] = v // Player has pointer-to-map fields; share by design
		}
	}
	if g.PublicData != nil {
		out.PublicData = deepCopyMap(g.PublicData)
	}
	if g.PrivateData != nil {
		out.PrivateData = deepCopyMap(g.PrivateData)
	}
	if g.PlayerData != nil {
		out.PlayerData = deepCopyMap(g.PlayerData)
	}
	return &out
}

func deepCopyMap(src map[string]interface{}) map[string]interface{} {
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = deepCopyValue(v)
	}
	return dst
}

func deepCopyValue(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		return deepCopyMap(t)
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, item := range t {
			out[i] = deepCopyValue(item)
		}
		return out
	default:
		return v
	}
}

func (s *MemoryStore) New(code string, script *models.Script, privateGame bool) (*models.Game, error) {
	if script == nil {
		return nil, errors.New("script is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.codes[code]; exists {
		return nil, fmt.Errorf("code %q: %w", code, repoErrors.ErrDuplicate)
	}
	now := time.Now()
	teams := make(map[string]models.Team, len(script.Teams))
	for k, v := range script.Teams {
		teams[k] = v
	}
	g := &models.Game{
		ID:          ids.New(),
		Version:     1,
		Code:        code,
		Teams:       teams,
		Players:     map[string]models.Player{},
		Stage:       script.Stage,
		PublicData:  map[string]interface{}{},
		PrivateData: map[string]interface{}{},
		PlayerData:  map[string]interface{}{},
		Private:     privateGame,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if script.PublicData != nil {
		g.PublicData = script.PublicData
	}
	if script.PrivateData != nil {
		g.PrivateData = script.PrivateData
	}
	s.games[g.ID] = g
	s.codes[code] = g.ID
	return copyGame(g), nil
}

func (s *MemoryStore) Get(id string) (*models.Game, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	g, ok := s.games[id]
	if !ok {
		return nil, fmt.Errorf("id %q: %w", id, repoErrors.ErrNotFound)
	}
	return copyGame(g), nil
}

func (s *MemoryStore) FindByCode(code string) (*models.Game, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.codes[code]
	if !ok {
		return nil, fmt.Errorf("code %q: %w", code, repoErrors.ErrNotFound)
	}
	return copyGame(s.games[id]), nil
}

func (s *MemoryStore) GetIDHex(code string) (*string, error) {
	g, err := s.FindByCode(code)
	if err != nil {
		return nil, err
	}
	return &g.ID, nil
}

func (s *MemoryStore) Exists(id string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.games[id]
	return ok, nil
}

func (s *MemoryStore) FindOpen(limit int) ([]*models.Game, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*models.Game, 0, limit)
	for _, g := range s.games {
		if g.Private {
			continue
		}
		out = append(out, copyGame(g))
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// applyDottedPath walks doc following segments; creates nested maps as
// needed. If del is true it removes the leaf, else it sets it to value.
// The doc is the JSON representation of a game — so nested structs like
// players (map[string]Player) appear as map[string]interface{} after ToMap.
func applyDottedPath(doc map[string]interface{}, path string, value interface{}, del bool) {
	segs := splitPath(path)
	cur := doc
	for i, seg := range segs {
		if i == len(segs)-1 {
			if del {
				delete(cur, seg)
			} else {
				cur[seg] = value
			}
			return
		}
		next, ok := cur[seg].(map[string]interface{})
		if !ok {
			next = make(map[string]interface{})
			cur[seg] = next
		}
		cur = next
	}
}

// splitPath splits a dotted path string into segments.
func splitPath(path string) []string {
	var segs []string
	start := 0
	for i := 0; i < len(path); i++ {
		if path[i] == '.' {
			segs = append(segs, path[start:i])
			start = i + 1
		}
	}
	segs = append(segs, path[start:])
	return segs
}

// fromMap re-hydrates a game from its map[string]interface{} JSON view.
// It unmarshals into a fresh Game before assigning to dst, because Go's
// json.Unmarshal merges into existing maps rather than replacing them —
// a subtle behaviour that would silently drop DeleteField's removals.
func fromMap(m map[string]interface{}, dst *models.Game) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	var fresh models.Game
	if err := json.Unmarshal(data, &fresh); err != nil {
		return err
	}
	*dst = fresh
	return nil
}

// publishFieldUpdate emits a partial-update event with the dotted-path field
// change, matching MongoStore.UpdateField's behaviour.
func (s *MemoryStore) publishFieldUpdate(id string, updated map[string]interface{}, removed []string) {
	if s.publisher == nil {
		return
	}
	updated, removed = events.SanitizeDelta(updated, removed)
	ev := events.ChangeEvent{
		ID:            id,
		OperationType: events.OpUpdate,
		Timestamp:     time.Now(),
		Collection:    collectionName,
		UpdatedFields: updated,
		RemovedFields: removed,
	}
	if !ev.HasChanges() {
		return
	}
	_ = s.publisher.Publish(s.ctx, ev)
}

// publishDiff computes the delta between the pre-mutation JSON snapshot and
// the newly saved game, mirroring MongoStore.publishDiff.
func (s *MemoryStore) publishDiff(id string, before map[string]interface{}, after *models.Game) {
	if s.publisher == nil {
		return
	}
	afterMap, err := events.ToMap(after)
	if err != nil {
		return
	}
	updated, removed := events.Diff(before, afterMap)
	s.publishFieldUpdate(id, updated, removed)
}

func (s *MemoryStore) Update(id string, upd *models.UpdateGame) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	g, ok := s.games[id]
	if !ok {
		return fmt.Errorf("id %q: %w", id, repoErrors.ErrNotFound)
	}
	before, err := events.ToMap(g)
	if err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}

	if upd.Teams != nil {
		g.Teams = *upd.Teams
	}
	if upd.Players != nil {
		g.Players = *upd.Players
	}
	if upd.Stage != nil {
		g.Stage = *upd.Stage
	}
	if upd.PublicData != nil {
		g.PublicData = upd.PublicData
	}
	if upd.PrivateData != nil {
		g.PrivateData = upd.PrivateData
	}
	if upd.PlayerData != nil {
		g.PlayerData = upd.PlayerData
	}
	g.Private = upd.Private
	g.UpdatedAt = time.Now()
	g.Version++

	s.publishDiff(id, before, g)
	return nil
}

func (s *MemoryStore) UpdateField(id string, key string, value interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	g, ok := s.games[id]
	if !ok {
		return fmt.Errorf("id %q: %w", id, repoErrors.ErrNotFound)
	}
	m, err := events.ToMap(g)
	if err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}
	applyDottedPath(m, key, value, false)
	if err := fromMap(m, g); err != nil {
		return fmt.Errorf("rehydrate: %w", err)
	}
	g.UpdatedAt = time.Now()
	g.Version++

	s.publishFieldUpdate(id, map[string]interface{}{key: value}, nil)
	return nil
}

func (s *MemoryStore) DeleteField(id string, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	g, ok := s.games[id]
	if !ok {
		return fmt.Errorf("id %q: %w", id, repoErrors.ErrNotFound)
	}
	m, err := events.ToMap(g)
	if err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}
	applyDottedPath(m, key, nil, true)
	if err := fromMap(m, g); err != nil {
		return fmt.Errorf("rehydrate: %w", err)
	}
	g.UpdatedAt = time.Now()
	g.Version++

	s.publishFieldUpdate(id, nil, []string{key})
	return nil
}

// Mutate uses mutation.Run for the retry-on-conflict CAS loop. The store's
// coarse RWMutex serialises writers within the process; the version fence
// (checked in save) makes the write safe even if the distributed lock's
// lease expires between load and save (matches MongoStore semantics).
func (s *MemoryStore) Mutate(id string, apply func(g *models.Game) error) error {
	var before map[string]interface{}

	return mutation.Run(
		s.ctx,
		s.locks,
		"game:"+id,
		func() (*models.Game, int64, error) {
			s.mu.RLock()
			defer s.mu.RUnlock()
			cur, ok := s.games[id]
			if !ok {
				return nil, 0, fmt.Errorf("id %q: %w", id, repoErrors.ErrNotFound)
			}
			g := copyGame(cur)
			beforeMap, err := events.ToMap(g)
			if err != nil {
				return nil, 0, err
			}
			before = beforeMap
			return g, cur.Version, nil
		},
		apply,
		func(g *models.Game, expectedVersion int64) (bool, error) {
			s.mu.Lock()
			defer s.mu.Unlock()
			cur, ok := s.games[id]
			if !ok {
				return false, fmt.Errorf("id %q: %w", id, repoErrors.ErrNotFound)
			}
			if cur.Version != expectedVersion {
				// Version drift — return false so mutation.Run reloads and retries.
				return false, nil
			}
			g.Version = expectedVersion + 1
			g.UpdatedAt = time.Now()
			s.games[id] = g
			s.publishDiff(id, before, g)
			return true, nil
		},
	)
}
