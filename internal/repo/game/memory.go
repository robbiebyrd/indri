package game

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/robbiebyrd/indri/internal/models"
	repoErrors "github.com/robbiebyrd/indri/internal/repo"
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
	ctx     context.Context
	locks   lock.Manager
	changes changePublisher

	mu    sync.RWMutex
	games map[string]*models.Game // id -> game
	codes map[string]string       // code -> id (unique)
}

var _ Storer = (*MemoryStore)(nil)

func NewMemoryStore(ctx context.Context, locks lock.Manager, publisher events.Publisher) (*MemoryStore, error) {
	if locks == nil {
		return nil, errors.New("locks is required")
	}
	if publisher == nil {
		return nil, errors.New("publisher is required")
	}
	return &MemoryStore{
		ctx:     ctx,
		locks:   locks,
		changes: changePublisher{ctx: ctx, publisher: publisher},
		games:   make(map[string]*models.Game),
		codes:   make(map[string]string),
	}, nil
}

// copyGame returns a deep copy sharing nothing mutable with g: every map,
// slice and pointer-to-map reachable from the game is re-created. The store
// hands copies to callers and to Mutate's apply, and keeps a copy of what it
// saves, so no one can edit stored state outside the lock, and a failed or
// aborted apply leaves it untouched. It copies field by field rather than
// through JSON because a JSON round trip would drop the json:"-" fields and
// turn empty-but-non-nil maps (omitempty) into nil ones that callers write to.
func copyGame(g *models.Game) *models.Game {
	if g == nil {
		return nil
	}
	out := *g
	if g.Teams != nil {
		out.Teams = make(map[string]models.Team, len(g.Teams))
		for k, v := range g.Teams {
			t := v
			t.PlayerIDs = copyStrings(v.PlayerIDs)
			t.PublicData = copyData(v.PublicData)
			t.PrivateData = copyData(v.PrivateData)
			t.PlayerData = copyPlayerData(v.PlayerData)
			out.Teams[k] = t
		}
	}
	if g.Players != nil {
		out.Players = make(map[string]models.Player, len(g.Players))
		for k, v := range g.Players {
			p := v
			p.PublicData = copyDataPtr(v.PublicData)
			p.PrivateData = copyDataPtr(v.PrivateData)
			out.Players[k] = p
		}
	}
	out.Stage = copyStage(g.Stage)
	out.PublicData = copyData(g.PublicData)
	out.PrivateData = copyData(g.PrivateData)
	out.PlayerData = copyData(g.PlayerData)
	return &out
}

func copyStage(st models.Stage) models.Stage {
	out := st
	out.SceneOrder = copyStrings(st.SceneOrder)
	if st.Scenes != nil {
		out.Scenes = make(map[string]models.Scene, len(st.Scenes))
		for k, v := range st.Scenes {
			sc := v
			sc.PublicData = copyDataPtr(v.PublicData)
			sc.PrivateData = copyDataPtr(v.PrivateData)
			sc.PlayerData = copyPlayerDataPtr(v.PlayerData)
			out.Scenes[k] = sc
		}
	}
	out.PublicData = copyData(st.PublicData)
	out.PrivateData = copyData(st.PrivateData)
	out.PlayerData = copyPlayerData(st.PlayerData)
	return out
}

func copyStrings(src []string) []string {
	if src == nil {
		return nil
	}
	return append([]string{}, src...)
}

// copyData deep-copies a data map, keeping nil as nil.
func copyData(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return nil
	}
	return deepCopyMap(src)
}

func copyDataPtr(src *map[string]interface{}) *map[string]interface{} {
	if src == nil {
		return nil
	}
	m := copyData(*src)
	return &m
}

func copyPlayerData(src map[string]map[string]interface{}) map[string]map[string]interface{} {
	if src == nil {
		return nil
	}
	dst := make(map[string]map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = copyData(v)
	}
	return dst
}

func copyPlayerDataPtr(src *map[string]map[string]interface{}) *map[string]map[string]interface{} {
	if src == nil {
		return nil
	}
	m := copyPlayerData(*src)
	return &m
}

func deepCopyMap(src map[string]interface{}) map[string]interface{} {
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = deepCopyValue(v)
	}
	return dst
}

// deepCopyValue copies the containers a data value can hold. JSON-decoded
// values (maps and []interface{}) take the fast path; any other map or slice
// a Go caller stored is copied by reflection so it is not shared either.
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
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Map:
		if rv.IsNil() {
			return v
		}
		out := reflect.MakeMapWithSize(rv.Type(), rv.Len())
		iter := rv.MapRange()
		for iter.Next() {
			out.SetMapIndex(iter.Key(), copiedElem(iter.Value(), rv.Type().Elem()))
		}
		return out.Interface()
	case reflect.Slice:
		if rv.IsNil() {
			return v
		}
		out := reflect.MakeSlice(rv.Type(), rv.Len(), rv.Len())
		for i := 0; i < rv.Len(); i++ {
			out.Index(i).Set(copiedElem(rv.Index(i), rv.Type().Elem()))
		}
		return out.Interface()
	default:
		return v
	}
}

// copiedElem deep-copies one map or slice element, keeping its static type.
func copiedElem(v reflect.Value, elemType reflect.Type) reflect.Value {
	c := deepCopyValue(v.Interface())
	if c == nil {
		return reflect.Zero(elemType)
	}
	return reflect.ValueOf(c)
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
	g, err := newGame(code, script, privateGame)
	if err != nil {
		return nil, err
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

// Exists reports whether a game with the given code exists.
func (s *MemoryStore) Exists(code string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.codes[code]
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

func (s *MemoryStore) Update(id string, upd *models.UpdateGame) error {
	return update(s, id, upd)
}

func (s *MemoryStore) UpdateField(id string, key string, value interface{}) error {
	return updateField(s, id, key, value)
}

func (s *MemoryStore) DeleteField(id string, key string) error {
	return deleteField(s, id, key)
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
			committed, err := s.commit(id, g, expectedVersion)
			if committed {
				// Published after the store lock is released: a subscriber
				// may read the game back (a keyframe), and Publish may block.
				// The mutation lock is still held, so events for one game
				// still go out in commit order.
				s.changes.diff(id, before, g)
			}
			return committed, err
		},
	)
}

// commit saves g if the stored version is still expectedVersion, reporting
// whether it did. It stores a copy: apply may have kept references into g, or
// put caller-owned maps in it.
func (s *MemoryStore) commit(id string, g *models.Game, expectedVersion int64) (bool, error) {
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
	s.games[id] = copyGame(g)
	return true, nil
}
