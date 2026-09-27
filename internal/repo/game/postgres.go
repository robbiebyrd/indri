package game

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib" // registers "pgx" driver for database/sql
	"github.com/robbiebyrd/indri/internal/models"
	repoErrors "github.com/robbiebyrd/indri/internal/repo"
	"github.com/robbiebyrd/indri/internal/services/events"
	"github.com/robbiebyrd/indri/internal/services/lock"
)

const postgresSchemaSQL = `
CREATE TABLE IF NOT EXISTS games (
    id      TEXT    PRIMARY KEY,
    code    TEXT    UNIQUE NOT NULL,
    version BIGINT  NOT NULL DEFAULT 1,
    private BOOLEAN NOT NULL DEFAULT FALSE,
    data    JSONB   NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_games_code    ON games (code);
CREATE INDEX IF NOT EXISTS idx_games_private ON games (private);
`

// PostgresStore is a PostgreSQL-backed game.Storer. Each game is stored as a
// JSONB blob alongside indexed scalar columns for efficient lookups.
type PostgresStore struct {
	ctx     context.Context
	db      *sql.DB
	locks   lock.Manager
	changes changePublisher
}

// TODO(postgres): reinstate after all Storer methods land in Task 7.
// var _ Storer = (*PostgresStore)(nil)

func NewPostgresStore(ctx context.Context, uri string, locks lock.Manager, publisher events.Publisher) (*PostgresStore, error) {
	if uri == "" {
		return nil, errors.New("postgres URI is required")
	}
	if locks == nil {
		return nil, errors.New("locks is required")
	}
	if publisher == nil {
		return nil, errors.New("publisher is required")
	}

	db, err := sql.Open("pgx", uri)
	if err != nil {
		return nil, fmt.Errorf("opening postgres connection: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("pinging postgres: %w", err)
	}

	if _, err := db.ExecContext(ctx, postgresSchemaSQL); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("running game schema DDL: %w", err)
	}

	return &PostgresStore{
		ctx:     ctx,
		db:      db,
		locks:   locks,
		changes: changePublisher{ctx: ctx, publisher: publisher},
	}, nil
}

// marshalGame serializes a game to JSON for storage in the JSONB column.
func marshalGame(g *models.Game) ([]byte, error) {
	return json.Marshal(g)
}

// unmarshalGame deserializes a game from the JSONB column.
func unmarshalGame(data []byte) (*models.Game, error) {
	var g models.Game
	if err := json.Unmarshal(data, &g); err != nil {
		return nil, err
	}
	return &g, nil
}

// isPgDuplicateKey reports whether err is a PostgreSQL unique-constraint violation.
// pgx wraps these as *pgconn.PgError with code "23505".
func isPgDuplicateKey(err error) bool {
	if err == nil {
		return false
	}
	// pgx encodes the SQLSTATE code in the error message; checking the string
	// is robust to import-path differences and avoids importing pgconn here.
	return strings.Contains(err.Error(), "23505") ||
		strings.Contains(err.Error(), "duplicate key value violates unique constraint")
}

func (s *PostgresStore) New(code string, script *models.Script, privateGame bool) (*models.Game, error) {
	if script == nil {
		return nil, errors.New("script is required")
	}
	g, err := newGame(code, script, privateGame)
	if err != nil {
		return nil, err
	}

	data, err := marshalGame(g)
	if err != nil {
		return nil, fmt.Errorf("marshaling game: %w", err)
	}

	_, err = s.db.ExecContext(s.ctx,
		`INSERT INTO games (id, code, version, private, data) VALUES ($1, $2, $3, $4, $5)`,
		g.ID, g.Code, g.Version, g.Private, data,
	)
	if err != nil {
		if isPgDuplicateKey(err) {
			return nil, fmt.Errorf("game with code %q already exists: %w", code, repoErrors.ErrDuplicate)
		}
		return nil, fmt.Errorf("inserting game: %w", err)
	}
	return s.Get(g.ID)
}

func (s *PostgresStore) Get(id string) (*models.Game, error) {
	var (
		data    []byte
		version int64
	)
	err := s.db.QueryRowContext(s.ctx,
		`SELECT data, version FROM games WHERE id = $1`, id,
	).Scan(&data, &version)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("id %q: %w", id, repoErrors.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("querying game: %w", err)
	}
	g, err := unmarshalGame(data)
	if err != nil {
		return nil, err
	}
	// models.Game.Version is json:"-" — restore from column.
	g.Version = version
	return g, nil
}

func (s *PostgresStore) FindByCode(gameCode string) (*models.Game, error) {
	var (
		data    []byte
		version int64
	)
	err := s.db.QueryRowContext(s.ctx,
		`SELECT data, version FROM games WHERE code = $1`, gameCode,
	).Scan(&data, &version)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("code %q: %w", gameCode, repoErrors.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("querying game by code: %w", err)
	}
	g, err := unmarshalGame(data)
	if err != nil {
		return nil, err
	}
	g.Version = version
	return g, nil
}

func (s *PostgresStore) GetIDHex(gameCode string) (*string, error) {
	g, err := s.FindByCode(gameCode)
	if err != nil {
		return nil, err
	}
	return &g.ID, nil
}

func (s *PostgresStore) Exists(id string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(s.ctx,
		`SELECT EXISTS(SELECT 1 FROM games WHERE id = $1)`, id,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking game existence: %w", err)
	}
	return exists, nil
}

func (s *PostgresStore) FindOpen(limit int) ([]*models.Game, error) {
	rows, err := s.db.QueryContext(s.ctx,
		`SELECT data, version FROM games WHERE private = FALSE LIMIT $1`, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("querying open games: %w", err)
	}
	defer rows.Close()

	var out []*models.Game
	for rows.Next() {
		var (
			data    []byte
			version int64
		)
		if err := rows.Scan(&data, &version); err != nil {
			return nil, fmt.Errorf("scanning open game: %w", err)
		}
		g, err := unmarshalGame(data)
		if err != nil {
			return nil, fmt.Errorf("unmarshaling game: %w", err)
		}
		g.Version = version // Game.Version is json:"-" — restore from column
		out = append(out, g)
	}
	return out, rows.Err()
}
