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
)

// sqlDocs is core's persistence port over a SQL database. One implementation
// serves both SQLite and PostgreSQL: the two differ only in how a placeholder
// is spelled, which dialect supplies.
//
// The game is stored whole, as JSON in a single column, with id, code, version
// and private lifted out beside it. Those four are the only things a query ever
// filters or fences on, so nothing else needs a column — and the document keeps
// the exact shape the rest of the package already works with, which is what
// lets every rule in core stay backend-independent.
//
// Version lives in its own column rather than only inside the JSON because the
// version fence is a conditional UPDATE: the database has to compare it without
// parsing the document.
type sqlDocs struct {
	db      *sql.DB
	dialect dialect
}

// dialect is the small amount that genuinely differs between SQLite and
// PostgreSQL. Anything larger than this belongs in sqlDocs, shared.
type dialect struct {
	// placeholder renders the n-th bind parameter, 1-based: "?" for SQLite,
	// "$1" for PostgreSQL.
	placeholder func(n int) string

	// isUniqueViolation reports whether err is the database's way of saying a
	// unique index rejected the write. Both drivers report it differently and
	// neither does so with a sentinel error.
	isUniqueViolation func(error) bool
}

// rebind renders query, replacing each "?" with the dialect's placeholder.
// Writing every statement once in "?" form and translating here keeps a single
// copy of each query.
func (d dialect) rebind(query string) string {
	if d.placeholder(1) == "?" {
		return query
	}

	var b strings.Builder

	n := 0

	for _, r := range query {
		if r == '?' {
			n++
			b.WriteString(d.placeholder(n))

			continue
		}

		b.WriteRune(r)
	}

	return b.String()
}

func (s sqlDocs) query(q string) string { return s.dialect.rebind(q) }

// encode renders the game as the JSON stored in the data column.
func encodeGame(g *models.Game) ([]byte, error) {
	raw, err := json.Marshal(g)
	if err != nil {
		return nil, fmt.Errorf("encoding game %q: %w", g.ID, err)
	}

	return raw, nil
}

// decode rebuilds a game from a stored row. The id, version and private flag
// come from their own columns rather than the document, so a row stays
// authoritative even if the two ever disagree.
func decodeGame(raw []byte, id string, version int64, private bool) (*models.Game, error) {
	var g models.Game
	if err := json.Unmarshal(raw, &g); err != nil {
		return nil, fmt.Errorf("decoding game %q: %w", id, err)
	}

	g.ID = id
	g.Version = version
	g.Private = private

	return &g, nil
}

func (s sqlDocs) insert(ctx context.Context, create models.CreateGame) (string, error) {
	g, err := gameFromCreate(create)
	if err != nil {
		return "", err
	}

	g.ID = create.ID
	if g.ID == "" {
		return "", errors.New("inserting game: the create document carries no id")
	}

	raw, err := encodeGame(g)
	if err != nil {
		return "", err
	}

	_, err = s.db.ExecContext(ctx, s.query(
		`INSERT INTO games (id, code, version, private, data) VALUES (?, ?, ?, ?, ?)`),
		g.ID, g.Code, g.Version, g.Private, string(raw),
	)
	if err != nil {
		// The unique code index rejected a concurrent create with the same
		// code. Both other backends phrase this the same way; a test that
		// asserts on the message must not care which store answered.
		if s.dialect.isUniqueViolation(err) {
			return "", fmt.Errorf("game with code %s already exists", create.Code)
		}

		return "", fmt.Errorf("inserting game %q: %w", g.ID, err)
	}

	return g.ID, nil
}

// scanGame reads one game from a row-returning query.
func (s sqlDocs) scanGame(row *sql.Row, describe string) (*models.Game, error) {
	var (
		id      string
		version int64
		private bool
		raw     []byte
	)

	switch err := row.Scan(&id, &version, &private, &raw); {
	case errors.Is(err, sql.ErrNoRows):
		return nil, fmt.Errorf("%s: %w", describe, errNotFound)
	case err != nil:
		return nil, fmt.Errorf("%s: %w", describe, err)
	}

	return decodeGame(raw, id, version, private)
}

const selectColumns = `SELECT id, version, private, data FROM games`

func (s sqlDocs) load(ctx context.Context, id string) (*models.Game, error) {
	return s.scanGame(
		s.db.QueryRowContext(ctx, s.query(selectColumns+` WHERE id = ?`), id),
		fmt.Sprintf("fetching game %q", id),
	)
}

func (s sqlDocs) findByCode(ctx context.Context, code string) (*models.Game, error) {
	return s.scanGame(
		s.db.QueryRowContext(ctx, s.query(selectColumns+` WHERE code = ?`), code),
		fmt.Sprintf("fetching game with code %q", code),
	)
}

func (s sqlDocs) findOpen(ctx context.Context, limit int) ([]*models.Game, error) {
	// Ordered by id so a caller sees a stable page, matching the in-memory
	// store, which sorts for the same reason.
	q := selectColumns + ` WHERE private = ? ORDER BY id`
	args := []interface{}{false}

	if limit > 0 {
		q += ` LIMIT ?`

		args = append(args, limit)
	}

	rows, err := s.db.QueryContext(ctx, s.query(q), args...)
	if err != nil {
		return nil, fmt.Errorf("listing open games: %w", err)
	}
	defer func() { _ = rows.Close() }()

	open := make([]*models.Game, 0, limit)

	for rows.Next() {
		var (
			id      string
			version int64
			private bool
			raw     []byte
		)

		if err := rows.Scan(&id, &version, &private, &raw); err != nil {
			return nil, fmt.Errorf("listing open games: %w", err)
		}

		g, err := decodeGame(raw, id, version, private)
		if err != nil {
			return nil, err
		}

		open = append(open, g)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing open games: %w", err)
	}

	return open, nil
}

func (s sqlDocs) existsByCode(ctx context.Context, code string) (bool, error) {
	var count int
	if err := s.db.QueryRowContext(ctx,
		s.query(`SELECT COUNT(*) FROM games WHERE code = ?`), code,
	).Scan(&count); err != nil {
		return false, fmt.Errorf("checking for game code %q: %w", code, err)
	}

	return count > 0, nil
}

// saveVersioned is the SQL version fence: the UPDATE carries the expected
// version in its WHERE clause, so a writer whose snapshot went stale changes no
// rows and is told it did not commit.
func (s sqlDocs) saveVersioned(
	ctx context.Context,
	id string,
	g *models.Game,
	expectedVersion int64,
) (bool, error) {
	g.UpdatedAt = time.Now()
	g.Version = expectedVersion + 1

	raw, err := encodeGame(g)
	if err != nil {
		return false, err
	}

	result, err := s.db.ExecContext(ctx, s.query(
		`UPDATE games SET data = ?, version = ?, code = ?, private = ? WHERE id = ? AND version = ?`),
		string(raw), g.Version, g.Code, g.Private, id, expectedVersion,
	)
	if err != nil {
		return false, fmt.Errorf("saving game %q: %w", id, err)
	}

	changed, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("saving game %q: %w", id, err)
	}

	if changed == 0 {
		// Either the fence rejected the write or the game is gone. Tell them
		// apart, because the first is a retry and the second is an error.
		if _, err := s.load(ctx, id); err != nil {
			return false, err
		}

		return false, nil
	}

	return true, nil
}

// mutateStored loads a game, hands it to edit, and writes it back with the
// version bumped. The read and the write run in one transaction so a
// concurrent writer cannot land between them.
//
// It backs every write that is not itself version-fenced — replace, setField,
// unsetField and setPlayerConnected — which on the document stores are single
// update operators. A SQL row holds the game as one JSON blob, so the
// equivalent is read-modify-write, and it has to be atomic to match.
func (s sqlDocs) mutateStored(ctx context.Context, id string, edit func(*models.Game) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("editing game %q: %w", id, err)
	}
	defer func() { _ = tx.Rollback() }()

	var (
		storedID string
		version  int64
		private  bool
		raw      []byte
	)

	switch err := tx.QueryRowContext(ctx,
		s.query(selectColumns+` WHERE id = ?`), id,
	).Scan(&storedID, &version, &private, &raw); {
	case errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("fetching game %q: %w", id, errNotFound)
	case err != nil:
		return fmt.Errorf("editing game %q: %w", id, err)
	}

	g, err := decodeGame(raw, storedID, version, private)
	if err != nil {
		return err
	}

	if err := edit(g); err != nil {
		return err
	}

	g.UpdatedAt = time.Now()
	g.Version = version + 1

	updated, err := encodeGame(g)
	if err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, s.query(
		`UPDATE games SET data = ?, version = ?, code = ?, private = ? WHERE id = ?`),
		string(updated), g.Version, g.Code, g.Private, id,
	); err != nil {
		return fmt.Errorf("editing game %q: %w", id, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("editing game %q: %w", id, err)
	}

	return nil
}

func (s sqlDocs) replace(ctx context.Context, id string, update *models.UpdateGame) error {
	return s.mutateStored(ctx, id, func(g *models.Game) error {
		applyUpdate(g, update)

		return nil
	})
}

func (s sqlDocs) setField(ctx context.Context, id string, key string, value interface{}) error {
	return s.mutateStored(ctx, id, func(g *models.Game) error {
		return setPath(g, key, value)
	})
}

func (s sqlDocs) unsetField(ctx context.Context, id string, key string) error {
	return s.mutateStored(ctx, id, func(g *models.Game) error {
		return unsetPath(g, key)
	})
}

func (s sqlDocs) setPlayerConnected(ctx context.Context, id string, userId string, connected bool) error {
	return s.mutateStored(ctx, id, func(g *models.Game) error {
		player, ok := g.Players[userId]
		if !ok {
			// Mirrors the MongoDB filter's $exists guard: a player who has been
			// removed must not be recreated by a connect.
			return fmt.Errorf("no player %v found in game %v", userId, id)
		}

		player.Connected = connected
		g.Players[userId] = player

		return nil
	})
}
