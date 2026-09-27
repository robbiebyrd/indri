package game

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/robbiebyrd/indri/internal/models"
	repoErrors "github.com/robbiebyrd/indri/internal/repo"
	"github.com/robbiebyrd/indri/internal/repo/ids"
	"github.com/robbiebyrd/indri/internal/services/events"
	"github.com/robbiebyrd/indri/internal/services/lock"
	"github.com/robbiebyrd/indri/internal/services/mutation"
)

// SQLiteStore is a SQLite-backed game.Storer. State is persisted in a single
// JSON blob per row. The write path mirrors MemoryStore: read-modify-write with
// a version fence in the UPDATE WHERE clause.
type SQLiteStore struct {
	ctx     context.Context
	db      *sql.DB
	locks   lock.Manager
	changes changePublisher
}

// TODO(sqlite): reinstate after all Storer methods land in Task 6.
// var _ Storer = (*SQLiteStore)(nil)

// NewSQLiteStore creates a SQLiteStore using an already-open *sql.DB. The caller
// is responsible for opening the DB via sqlite.Open, which runs the DDL.
func NewSQLiteStore(ctx context.Context, db *sql.DB, locks lock.Manager, publisher events.Publisher) (*SQLiteStore, error) {
	if db == nil {
		return nil, errors.New("db is required")
	}
	if locks == nil {
		return nil, errors.New("locks is required")
	}
	if publisher == nil {
		return nil, errors.New("publisher is required")
	}
	return &SQLiteStore{
		ctx:     ctx,
		db:      db,
		locks:   locks,
		changes: changePublisher{ctx: ctx, publisher: publisher},
	}, nil
}

// marshalGameText serializes a game to the TEXT blob stored in the SQLite data column.
func marshalGameText(g *models.Game) (string, error) {
	b, err := json.Marshal(g)
	if err != nil {
		return "", fmt.Errorf("marshal game: %w", err)
	}
	return string(b), nil
}

// unmarshalGameText deserializes a game from the TEXT blob in the SQLite data column.
func unmarshalGameText(data string) (*models.Game, error) {
	var g models.Game
	if err := json.Unmarshal([]byte(data), &g); err != nil {
		return nil, fmt.Errorf("unmarshal game: %w", err)
	}
	return &g, nil
}

func (s *SQLiteStore) New(code string, script *models.Script, privateGame bool) (*models.Game, error) {
	if script == nil {
		return nil, errors.New("script is required")
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

	blob, err := marshalGameText(g)
	if err != nil {
		return nil, err
	}

	private := 0
	if privateGame {
		private = 1
	}
	_, err = s.db.ExecContext(s.ctx,
		`INSERT INTO games (id, code, version, private, data) VALUES (?, ?, ?, ?, ?)`,
		g.ID, code, g.Version, private, blob,
	)
	if err != nil {
		if isSQLiteConstraintUnique(err) {
			return nil, fmt.Errorf("game with code %q: %w", code, repoErrors.ErrDuplicate)
		}
		return nil, fmt.Errorf("insert game: %w", err)
	}
	return g, nil
}

func (s *SQLiteStore) Get(id string) (*models.Game, error) {
	var (
		data    string
		version int64
	)
	err := s.db.QueryRowContext(s.ctx,
		`SELECT data, version FROM games WHERE id = ?`, id,
	).Scan(&data, &version)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("id %q: %w", id, repoErrors.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get game: %w", err)
	}
	g, err := unmarshalGameText(data)
	if err != nil {
		return nil, err
	}
	g.Version = version // Game.Version is json:"-" — restore from column
	return g, nil
}

func (s *SQLiteStore) FindByCode(gameCode string) (*models.Game, error) {
	var (
		data    string
		version int64
	)
	err := s.db.QueryRowContext(s.ctx,
		`SELECT data, version FROM games WHERE code = ?`, gameCode,
	).Scan(&data, &version)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("code %q: %w", gameCode, repoErrors.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("find game by code: %w", err)
	}
	g, err := unmarshalGameText(data)
	if err != nil {
		return nil, err
	}
	g.Version = version
	return g, nil
}

func (s *SQLiteStore) GetIDHex(gameCode string) (*string, error) {
	g, err := s.FindByCode(gameCode)
	if err != nil {
		return nil, err
	}
	return &g.ID, nil
}

func (s *SQLiteStore) Exists(id string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(s.ctx,
		`SELECT COUNT(*) FROM games WHERE id = ?`, id,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("exists game: %w", err)
	}
	return count > 0, nil
}

func (s *SQLiteStore) FindOpen(limit int) ([]*models.Game, error) {
	rows, err := s.db.QueryContext(s.ctx,
		`SELECT data, version FROM games WHERE private = 0 LIMIT ?`, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("find open games: %w", err)
	}
	defer rows.Close()

	var out []*models.Game
	for rows.Next() {
		var (
			data    string
			version int64
		)
		if err := rows.Scan(&data, &version); err != nil {
			return nil, fmt.Errorf("scan game: %w", err)
		}
		g, err := unmarshalGameText(data)
		if err != nil {
			return nil, err
		}
		g.Version = version // Game.Version is json:"-" — restore from column
		out = append(out, g)
	}
	return out, rows.Err()
}

// isSQLiteConstraintUnique reports whether err is a SQLite UNIQUE constraint
// violation. modernc.org/sqlite surfaces this as an error message containing
// "UNIQUE constraint failed".
func isSQLiteConstraintUnique(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// saveWithVersion performs the version-fenced UPDATE. Returns (true, nil) on
// success, (false, nil) if the version has drifted (triggering mutation.Run
// retry), or (false, err) on a hard error.
func (s *SQLiteStore) saveWithVersion(g *models.Game, expectedVersion int64) (bool, error) {
	g.Version = expectedVersion + 1
	g.UpdatedAt = time.Now()

	blob, err := marshalGameText(g)
	if err != nil {
		return false, err
	}
	private := 0
	if g.Private {
		private = 1
	}
	result, err := s.db.ExecContext(s.ctx,
		`UPDATE games SET data = ?, version = ?, private = ? WHERE id = ? AND version = ?`,
		blob, g.Version, private, g.ID, expectedVersion,
	)
	if err != nil {
		return false, fmt.Errorf("save game: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("rows affected: %w", err)
	}
	return n == 1, nil
}

// loadWithVersion reads the game and its current version for use in Mutate's
// CAS loop.
func (s *SQLiteStore) loadWithVersion(id string) (*models.Game, int64, error) {
	var (
		data    string
		version int64
	)
	err := s.db.QueryRowContext(s.ctx,
		`SELECT data, version FROM games WHERE id = ?`, id,
	).Scan(&data, &version)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, fmt.Errorf("id %q: %w", id, repoErrors.ErrNotFound)
	}
	if err != nil {
		return nil, 0, fmt.Errorf("load game: %w", err)
	}
	g, err := unmarshalGameText(data)
	if err != nil {
		return nil, 0, err
	}
	// models.Game.Version is tagged json:"-", so json.Unmarshal always leaves
	// it at 0. Restore it from the scalar column so callers of apply()
	// (including delta diffs) see the correct version.
	g.Version = version
	return g, version, nil
}

func (s *SQLiteStore) Update(id string, upd *models.UpdateGame) error {
	g, version, err := s.loadWithVersion(id)
	if err != nil {
		return err
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

	committed, err := s.saveWithVersion(g, version)
	if err != nil {
		return err
	}
	if !committed {
		return fmt.Errorf("id %q: %w", id, repoErrors.ErrConflict)
	}
	s.changes.diff(id, before, g)
	return nil
}

func (s *SQLiteStore) UpdateField(id string, key string, value interface{}) error {
	g, version, err := s.loadWithVersion(id)
	if err != nil {
		return err
	}
	m, err := events.ToMap(g)
	if err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}
	applyDottedPath(m, key, value, false)
	if err := fromMap(m, g); err != nil {
		return fmt.Errorf("rehydrate: %w", err)
	}

	committed, err := s.saveWithVersion(g, version)
	if err != nil {
		return err
	}
	if !committed {
		return fmt.Errorf("id %q: %w", id, repoErrors.ErrConflict)
	}
	s.changes.field(id, map[string]interface{}{key: value}, nil)
	return nil
}

func (s *SQLiteStore) DeleteField(id string, key string) error {
	g, version, err := s.loadWithVersion(id)
	if err != nil {
		return err
	}
	m, err := events.ToMap(g)
	if err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}
	applyDottedPath(m, key, nil, true)
	if err := fromMap(m, g); err != nil {
		return fmt.Errorf("rehydrate: %w", err)
	}

	committed, err := s.saveWithVersion(g, version)
	if err != nil {
		return err
	}
	if !committed {
		return fmt.Errorf("id %q: %w", id, repoErrors.ErrConflict)
	}
	s.changes.field(id, nil, []string{key})
	return nil
}

// Mutate uses mutation.Run for the optimistic retry loop. The load/save
// callbacks do not hold any in-process lock — SQLite's WAL mode handles
// concurrent reads, and the version fence in saveWithVersion prevents lost
// updates. The distributed lock.Manager (passed via clients) serialises
// concurrent callers across goroutines within the process.
func (s *SQLiteStore) Mutate(id string, apply func(g *models.Game) error) error {
	var before map[string]interface{}

	return mutation.Run(
		s.ctx,
		s.locks,
		"game:"+id,
		func() (*models.Game, int64, error) {
			g, version, err := s.loadWithVersion(id)
			if err != nil {
				return nil, 0, err
			}
			beforeMap, err := events.ToMap(g)
			if err != nil {
				return nil, 0, err
			}
			before = beforeMap
			return g, version, nil
		},
		apply,
		func(g *models.Game, expectedVersion int64) (bool, error) {
			committed, err := s.saveWithVersion(g, expectedVersion)
			if err != nil {
				return false, err
			}
			if committed {
				s.changes.diff(id, before, g)
			}
			return committed, nil
		},
	)
}
