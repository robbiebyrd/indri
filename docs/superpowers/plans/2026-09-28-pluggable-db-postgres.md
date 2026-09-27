# Pluggable DB Backends — PostgreSQL Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a PostgreSQL backend for all three repo packages (`game`, `user`, `session`), wire it into the injector, and add the `postgresUri` env var and JSON config key. The result: `INDRI_DB_BACKEND=postgres` starts a fully functional server backed by PostgreSQL with zero other code changes.

**Architecture:** Each `internal/repo/<entity>/` package gains a `PostgresStore` struct. The JSONB-blob approach is used throughout: each entity is stored as a single `JSONB data` column plus indexed columns for lookups (`id`, `code`, `version`, etc.). This preserves parity with the MongoDB and in-memory backends — all reads and writes operate on the full JSON-encoded struct, and `events.Diff` continues to work identically.

**Tech Stack:** Go 1.25, `github.com/jackc/pgx/v5` with its `stdlib` sub-package (`pgx/v5/stdlib`) for `database/sql` compatibility. pgx is chosen over `lib/pq` because it is actively maintained, supports the modern `pgconn` error types needed to detect duplicate-key violations, and its `stdlib` adapter lets us use the standard `database/sql` API that the rest of the codebase can follow without importing pgx-specific types in every file.

**Schema:** JSONB-blob approach. Each table stores `id TEXT PRIMARY KEY`, a small number of indexed scalar columns for efficient lookups, `version BIGINT NOT NULL DEFAULT 1` for the version fence, and `data JSONB NOT NULL` for the full struct. This avoids a complex normalised schema that would diverge from the memory/mongo approach, and PostgreSQL's JSONB supports GIN indexes for nested lookups if ever needed.

**Write strategy for Mutate:** `UPDATE ... WHERE id = $1 AND version = $2` checking `RowsAffected`. No explicit `SELECT ... FOR UPDATE` transaction is used — a single statement is atomic in PostgreSQL, and this matches the MemoryStore version-fence pattern.

**UpdateField / DeleteField:** Read-modify-write the full JSON document in Go (same as MemoryStore), rather than using `jsonb_set` / `jsonb - 'key'`. This keeps the delta-computation path identical across all backends and avoids SQL expressions that would have to replicate the dotted-path logic of `applyDottedPath`.

**Testing:** Integration tests are guarded on `INDRI_TEST_POSTGRES_URI`. When unset, all Postgres tests skip. A Docker one-liner is included below for local dev.

**Spec:** `docs/superpowers/specs/2026-09-27-pluggable-database-backends-design.md`

**Ground truth for types.** Before writing any code an executor MUST read these files:

- `internal/models/game.go` — `Game{ID string, Version int64, Code string, Private bool, Teams map[string]Team, Players map[string]Player, Stage Stage, PublicData map[string]interface{}, PrivateData map[string]interface{}, PlayerData map[string]interface{}, CreatedAt time.Time, UpdatedAt time.Time}`
- `internal/models/user.go` — `User{ID string, Email string, Name string, DisplayName *string, Password *string, Score *int, PublicData map[string]interface{}, PrivateData map[string]interface{}}`. `CreateUser{Email, Name string, DisplayName *string, Password *string}`. No `ID` on `CreateUser`.
- `internal/models/session.go` — `Session{ID string, Token string, GameID *string, UserID *string, TeamID *string}`. `CreateSession{Token, GameID, UserID, TeamID string}`.
- `internal/models/player.go` — `Player{Name string, Score int, Connected bool, Host bool, Controller bool, PublicData *map[string]interface{}, PrivateData *map[string]interface{}}`. **No UserID or DisplayName on Player.**
- `internal/models/team.go` — `Team{Name string, PlayerIDs []string, PublicData map[string]interface{}, PrivateData map[string]interface{}, PlayerData map[string]map[string]interface{}}`. **No ID on Team.** Team membership is `PlayerIDs []string` (slice of userIDs).
- `internal/services/mutation/mutation.go` — `Run[T](ctx, mgr, key, load func() (*T, int64, error), apply func(*T) error, save func(*T, int64) (bool, error)) error`.

---

## Local dev Docker one-liner

```bash
docker run --rm -d \
  -e POSTGRES_USER=indri \
  -e POSTGRES_PASSWORD=indri \
  -e POSTGRES_DB=indri \
  -p 5432:5432 \
  --name indri-postgres \
  postgres:15-alpine
export INDRI_TEST_POSTGRES_URI="postgres://indri:indri@localhost:5432/indri?sslmode=disable"
```

---

## Caveat: `Game.Version` is `json:"-"`

`models.Game.Version` is tagged `json:"-"` — `json.Marshal` drops it and `json.Unmarshal` never fills it in. The version is the CAS fence for `Mutate`, so every Postgres read path MUST select the scalar `version` column alongside the JSONB `data` and assign it back onto the unmarshalled game (`g.Version = version`). The code snippets in Tasks 3–4 already do this; when adding new read paths, follow the same pattern. The scalar column is the sole durable source of truth; the JSON blob's version field would always be 0 and must never be trusted.

## Caveat: no redeclarations across the package

`internal/repo/user/memory.go` already defines `matchUserField`; `internal/repo/session/memory.go` already defines `matchSessionField`. Since Postgres stores live in the same packages, DO NOT redeclare these — reuse them. The Postgres store files must only introduce truly new helpers.

---

## File Map

| File | Action | Responsibility |
|---|---|---|
| `go.mod`, `go.sum` | Modify | Add `github.com/jackc/pgx/v5` |
| `internal/repo/env/env.go` | Modify | Add `PostgresURI` field |
| `example/server.json` | Modify | Add `"postgresUri": ""` key |
| `internal/repo/game/postgres.go` | Create | `PostgresStore` struct, constructor (runs DDL), read ops |
| `internal/repo/game/postgres_player.go` | Create | Player methods on `PostgresStore` |
| `internal/repo/game/postgres_team.go` | Create | Team methods on `PostgresStore` |
| `internal/repo/game/postgres_host.go` | Create | Host methods; restore Storer assertion |
| `internal/repo/game/postgres_test.go` | Create | Integration tests guarded on `INDRI_TEST_POSTGRES_URI` |
| `internal/repo/user/postgres.go` | Create | `PostgresStore` for users |
| `internal/repo/user/postgres_test.go` | Create | Integration tests |
| `internal/repo/session/postgres.go` | Create | `PostgresStore` for sessions |
| `internal/repo/session/postgres_test.go` | Create | Integration tests |
| `internal/injector/repos.go` | Modify | Add `"postgres"` case |

---

## Task 1: Add pgx/v5 dependency

**Files:** `go.mod`, `go.sum`

- [ ] **Step 1.1 — Add the dependency**

```bash
go get github.com/jackc/pgx/v5
```

- [ ] **Step 1.2 — Verify the build still passes**

```bash
go build ./...
```

Expected: PASS (no new files reference pgx yet).

- [ ] **Step 1.3 — Commit**

```bash
git status
git add go.mod go.sum
git commit -m "chore(deps): add pgx/v5 for PostgreSQL backend"
```

---

## Task 2: Add `postgresUri` env var and config key

**Files:**
- Modify: `internal/repo/env/env.go`
- Modify: `example/server.json`

- [ ] **Step 2.1 — Add `PostgresURI` to `Vars`**

In `internal/repo/env/env.go`, add a field after `DBBackend`:

```go
PostgresURI string `default:"" envconfig:"POSTGRES_URI" json:"postgresUri" flag:"postgres-uri"`
```

The `json:"postgresUri"` tag must be a flat top-level key — `applyJSONConfigToEnv` does not support nested JSON.

- [ ] **Step 2.2 — Update the example config**

In `example/server.json`, add after `"dbBackend"`:

```json
"postgresUri": "",
```

- [ ] **Step 2.3 — Build and test env package**

```bash
go build ./...
go test ./internal/repo/env/
```

Expected: PASS.

- [ ] **Step 2.4 — Commit**

```bash
git status
git add internal/repo/env/env.go example/server.json
git commit -m "feat(env): add postgresUri config field"
```

---

## Task 3: `game.PostgresStore` — struct, constructor, DDL, and read operations

The constructor opens a `*sql.DB` pool and runs the schema DDL idempotently (using `CREATE TABLE IF NOT EXISTS`). Read operations use `SELECT data FROM games WHERE ...` and unmarshal JSON.

**Files:**
- Create: `internal/repo/game/postgres.go`
- Create: `internal/repo/game/postgres_test.go`

### DDL

The games table schema (inline constant, not a separate `.sql` file — keeps the package self-contained):

```sql
CREATE TABLE IF NOT EXISTS games (
    id      TEXT        PRIMARY KEY,
    code    TEXT        UNIQUE NOT NULL,
    version BIGINT      NOT NULL DEFAULT 1,
    private BOOLEAN     NOT NULL DEFAULT FALSE,
    data    JSONB       NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_games_code    ON games (code);
CREATE INDEX IF NOT EXISTS idx_games_private ON games (private);
```

`id`, `code`, `version`, and `private` are scalar columns redundant with `data` — they exist only to enable indexed lookups without a full JSONB scan. On every write, these columns are kept in sync with the corresponding fields inside `data`.

- [ ] **Step 3.1 — Write the failing constructor test**

Create `internal/repo/game/postgres_test.go`:

```go
package game

import (
	"context"
	"errors"
	"os"
	"testing"

	repoErrors "github.com/robbiebyrd/indri/internal/repo"
	"github.com/robbiebyrd/indri/internal/services/events"
	"github.com/robbiebyrd/indri/internal/services/lock"
)

func postgresURI(t *testing.T) string {
	t.Helper()
	uri := os.Getenv("INDRI_TEST_POSTGRES_URI")
	if uri == "" {
		t.Skip("INDRI_TEST_POSTGRES_URI not set; skipping Postgres integration test")
	}
	return uri
}

func newPostgresFixture(t *testing.T) *PostgresStore {
	t.Helper()
	store, err := NewPostgresStore(
		context.Background(),
		postgresURI(t),
		lock.NewInProcess(),
		events.NewInProcess(),
	)
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}
	t.Cleanup(func() { _ = store.db.Exec("TRUNCATE TABLE games") })
	return store
}

func TestNewPostgresStore_ReturnsUsableStore(t *testing.T) {
	store := newPostgresFixture(t)
	if store == nil {
		t.Fatal("NewPostgresStore returned nil without error")
	}
}
```

Note: `store.db.Exec` — the `PostgresStore` struct exposes `db *sql.DB` as a field. Access is package-internal (test is in `package game`).

- [ ] **Step 3.2 — Run to confirm FAIL**

```bash
INDRI_TEST_POSTGRES_URI="postgres://indri:indri@localhost:5432/indri?sslmode=disable" \
  go test ./internal/repo/game/ -run TestNewPostgresStore
```

Expected: FAIL with `undefined: NewPostgresStore`.

- [ ] **Step 3.3 — Implement the struct, constructor, and read ops**

Create `internal/repo/game/postgres.go`:

```go
package game

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // registers "pgx" driver for database/sql
	"github.com/robbiebyrd/indri/internal/models"
	repoErrors "github.com/robbiebyrd/indri/internal/repo"
	"github.com/robbiebyrd/indri/internal/repo/ids"
	"github.com/robbiebyrd/indri/internal/services/events"
	"github.com/robbiebyrd/indri/internal/services/lock"
	"github.com/robbiebyrd/indri/internal/services/mutation"
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
	ctx       context.Context
	db        *sql.DB
	locks     lock.Manager
	publisher events.Publisher
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
		ctx:       ctx,
		db:        db,
		locks:     locks,
		publisher: publisher,
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
```

- [ ] **Step 3.4 — Run to confirm PASS**

```bash
INDRI_TEST_POSTGRES_URI="postgres://indri:indri@localhost:5432/indri?sslmode=disable" \
  go test ./internal/repo/game/ -run TestNewPostgresStore -v
```

- [ ] **Step 3.5 — Write failing read tests**

Append to `internal/repo/game/postgres_test.go`:

```go
func TestPostgresStore_NewGame_AssignsIDAndCode(t *testing.T) {
	store := newPostgresFixture(t)
	g, err := store.New("ABCD", makeScript(), false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if g.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if g.Code != "ABCD" || g.Version != 1 {
		t.Errorf("Code=%q Version=%d; want ABCD, 1", g.Code, g.Version)
	}
}

func TestPostgresStore_NewGame_DuplicateCode_ReturnsErrDuplicate(t *testing.T) {
	store := newPostgresFixture(t)
	if _, err := store.New("ABCD", makeScript(), false); err != nil {
		t.Fatalf("first New: %v", err)
	}
	_, err := store.New("ABCD", makeScript(), false)
	if !errors.Is(err, repoErrors.ErrDuplicate) {
		t.Fatalf("expected ErrDuplicate, got %v", err)
	}
}

func TestPostgresStore_Get_UnknownID_ReturnsErrNotFound(t *testing.T) {
	store := newPostgresFixture(t)
	_, err := store.Get("no-such-id")
	if !errors.Is(err, repoErrors.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestPostgresStore_FindByCode(t *testing.T) {
	store := newPostgresFixture(t)
	created, _ := store.New("WXYZ", makeScript(), false)
	got, err := store.FindByCode("WXYZ")
	if err != nil {
		t.Fatalf("FindByCode: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("ID: want %q, got %q", created.ID, got.ID)
	}
}

func TestPostgresStore_Exists(t *testing.T) {
	store := newPostgresFixture(t)
	created, _ := store.New("ABCD", makeScript(), false)
	exists, _ := store.Exists(created.ID)
	if !exists {
		t.Errorf("expected exists=true for created id")
	}
	exists, _ = store.Exists("nope")
	if exists {
		t.Errorf("expected exists=false for unknown id")
	}
}

func TestPostgresStore_FindOpen_ExcludesPrivate(t *testing.T) {
	store := newPostgresFixture(t)
	_, _ = store.New("PUB1", makeScript(), false)
	_, _ = store.New("PRV1", makeScript(), true)
	games, err := store.FindOpen(10)
	if err != nil {
		t.Fatalf("FindOpen: %v", err)
	}
	if len(games) != 1 || games[0].Code != "PUB1" {
		t.Fatalf("want [PUB1], got %v", games)
	}
}
```

Note: `makeScript()` and `scriptWithTeams()` are already defined in `memory_test.go` in the same package. Do not redefine them.

- [ ] **Step 3.6 — Run to confirm PASS**

```bash
INDRI_TEST_POSTGRES_URI="postgres://indri:indri@localhost:5432/indri?sslmode=disable" \
  go test ./internal/repo/game/ -run TestPostgresStore -v
```

- [ ] **Step 3.7 — Full build check**

```bash
go build ./...
```

Expected: PASS (assertion is commented out).

- [ ] **Step 3.8 — Commit**

```bash
git status
git add internal/repo/game/postgres.go internal/repo/game/postgres_test.go
git commit -m "feat(game/postgres): struct, constructor, DDL, and read operations"
```

---

## Task 4: `game.PostgresStore` — write path (Update, UpdateField, DeleteField, Mutate)

The write path uses the same read-modify-write pattern as `MemoryStore`: load the full JSON, modify the struct in Go, write the full JSON back. `Mutate` uses `mutation.Run` with an `UPDATE ... WHERE id = $1 AND version = $2` version fence.

The helper functions `applyDottedPath`, `splitPath`, `fromMap`, `marshalGame`, `unmarshalGame` already exist in `memory.go` / `postgres.go` within the same package. Do not duplicate them.

**Files:**
- Modify: `internal/repo/game/postgres.go`
- Modify: `internal/repo/game/postgres_test.go`

- [ ] **Step 4.1 — Write failing tests**

Append to `internal/repo/game/postgres_test.go`:

```go
func TestPostgresStore_Update_BumpsVersion(t *testing.T) {
	store := newPostgresFixture(t)
	g, _ := store.New("ABCD", makeScript(), false)
	upd := &models.UpdateGame{Private: true}
	if err := store.Update(g.ID, upd); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ := store.Get(g.ID)
	if !got.Private || got.Version != 2 {
		t.Errorf("Version=%d Private=%v; want 2, true", got.Version, got.Private)
	}
}

func TestPostgresStore_UpdateField(t *testing.T) {
	store := newPostgresFixture(t)
	g, _ := store.New("ABCD", makeScript(), false)
	if err := store.UpdateField(g.ID, "data.foo", "bar"); err != nil {
		t.Fatalf("UpdateField: %v", err)
	}
	got, _ := store.Get(g.ID)
	if v, _ := got.PublicData["foo"]; v != "bar" {
		t.Errorf("data.foo = %v; want bar", v)
	}
}

func TestPostgresStore_DeleteField(t *testing.T) {
	store := newPostgresFixture(t)
	g, _ := store.New("ABCD", makeScript(), false)
	_ = store.UpdateField(g.ID, "data.foo", "bar")
	if err := store.DeleteField(g.ID, "data.foo"); err != nil {
		t.Fatalf("DeleteField: %v", err)
	}
	got, _ := store.Get(g.ID)
	if _, still := got.PublicData["foo"]; still {
		t.Errorf("data.foo not deleted")
	}
}

func TestPostgresStore_Mutate_SuccessfulApply(t *testing.T) {
	store := newPostgresFixture(t)
	g, _ := store.New("ABCD", makeScript(), false)
	err := store.Mutate(g.ID, func(game *models.Game) error {
		game.Private = true
		return nil
	})
	if err != nil {
		t.Fatalf("Mutate: %v", err)
	}
	got, _ := store.Get(g.ID)
	if !got.Private || got.Version != 2 {
		t.Errorf("Mutate did not apply or bump version: %+v", got)
	}
}

func TestPostgresStore_Mutate_UnknownID(t *testing.T) {
	store := newPostgresFixture(t)
	err := store.Mutate("no-such-id", func(g *models.Game) error { return nil })
	if !errors.Is(err, repoErrors.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
```

- [ ] **Step 4.2 — Run to confirm FAIL**

```bash
INDRI_TEST_POSTGRES_URI="postgres://indri:indri@localhost:5432/indri?sslmode=disable" \
  go test ./internal/repo/game/ -run TestPostgresStore_Update -v
```

- [ ] **Step 4.3 — Implement write path**

Append to `internal/repo/game/postgres.go`:

```go
// saveGame writes the full game back to the database, updating the scalar
// index columns (id, code, version, private) alongside the JSONB blob.
func (s *PostgresStore) saveGame(g *models.Game) error {
	data, err := marshalGame(g)
	if err != nil {
		return fmt.Errorf("marshaling game: %w", err)
	}
	_, err = s.db.ExecContext(s.ctx,
		`UPDATE games SET code = $2, version = $3, private = $4, data = $5 WHERE id = $1`,
		g.ID, g.Code, g.Version, g.Private, data,
	)
	return err
}

// saveWithVersion is the version-fenced write used by Mutate. It returns
// (true, nil) on a successful commit, (false, nil) when another writer
// already incremented the version (triggers a retry in mutation.Run), or
// (false, err) on a real database error.
func (s *PostgresStore) saveWithVersion(g *models.Game, expectedVersion int64) (bool, error) {
	g.Version = expectedVersion + 1
	g.UpdatedAt = time.Now()
	data, err := marshalGame(g)
	if err != nil {
		return false, fmt.Errorf("marshaling game: %w", err)
	}
	result, err := s.db.ExecContext(s.ctx,
		`UPDATE games SET code = $3, version = $4, private = $5, data = $6
		 WHERE id = $1 AND version = $2`,
		g.ID, expectedVersion, g.Code, g.Version, g.Private, data,
	)
	if err != nil {
		return false, fmt.Errorf("saving game with version fence: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}

// publishFieldUpdate emits a partial-update event, mirroring MemoryStore.
func (s *PostgresStore) publishFieldUpdate(id string, updated map[string]interface{}, removed []string) {
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

// publishDiff computes and publishes the delta between a pre-mutation JSON
// snapshot and the updated game.
func (s *PostgresStore) publishDiff(id string, before map[string]interface{}, after *models.Game) {
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

func (s *PostgresStore) Update(id string, upd *models.UpdateGame) error {
	g, err := s.Get(id)
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
	g.UpdatedAt = time.Now()
	g.Version++

	if err := s.saveGame(g); err != nil {
		return fmt.Errorf("saving updated game: %w", err)
	}
	s.publishDiff(id, before, g)
	return nil
}

func (s *PostgresStore) UpdateField(id string, key string, value interface{}) error {
	g, err := s.Get(id)
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
	g.UpdatedAt = time.Now()
	g.Version++

	if err := s.saveGame(g); err != nil {
		return fmt.Errorf("saving field update: %w", err)
	}
	s.publishFieldUpdate(id, map[string]interface{}{key: value}, nil)
	return nil
}

func (s *PostgresStore) DeleteField(id string, key string) error {
	g, err := s.Get(id)
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
	g.UpdatedAt = time.Now()
	g.Version++

	if err := s.saveGame(g); err != nil {
		return fmt.Errorf("saving field delete: %w", err)
	}
	s.publishFieldUpdate(id, nil, []string{key})
	return nil
}

// Mutate uses mutation.Run for the retry-on-conflict CAS loop. The version
// fence in saveWithVersion guarantees writes committed under an expired lock
// cannot overwrite concurrent changes.
func (s *PostgresStore) Mutate(id string, apply func(g *models.Game) error) error {
	var before map[string]interface{}

	return mutation.Run(
		s.ctx,
		s.locks,
		"game:"+id,
		func() (*models.Game, int64, error) {
			g, err := s.Get(id)
			if err != nil {
				return nil, 0, err
			}
			beforeMap, err := events.ToMap(g)
			if err != nil {
				return nil, 0, err
			}
			before = beforeMap
			return g, g.Version, nil
		},
		apply,
		func(g *models.Game, expectedVersion int64) (bool, error) {
			committed, err := s.saveWithVersion(g, expectedVersion)
			if err != nil {
				return false, err
			}
			if committed {
				s.publishDiff(id, before, g)
			}
			return committed, nil
		},
	)
}
```

- [ ] **Step 4.4 — Run to confirm PASS**

```bash
INDRI_TEST_POSTGRES_URI="postgres://indri:indri@localhost:5432/indri?sslmode=disable" \
  go test ./internal/repo/game/ -run TestPostgresStore -v
```

- [ ] **Step 4.5 — Full build**

```bash
go build ./...
```

- [ ] **Step 4.6 — Commit**

```bash
git status
git add internal/repo/game/postgres.go internal/repo/game/postgres_test.go
git commit -m "feat(game/postgres): add write path (Update, UpdateField, DeleteField, Mutate)"
```

---

## Task 5: `game.PostgresStore` — Player operations

Player ops all go through `Mutate` — the version-fenced CAS loop. `Game.Players` is `map[string]models.Player` keyed by userID. `models.Player` has no `UserID` field; the userID is the map key.

**Files:**
- Create: `internal/repo/game/postgres_player.go`
- Modify: `internal/repo/game/postgres_test.go`

- [ ] **Step 5.1 — Write failing tests**

Append to `internal/repo/game/postgres_test.go`:

```go
func TestPostgresStore_AddPlayer_ThenHasPlayer(t *testing.T) {
	store := newPostgresFixture(t)
	g, _ := store.New("ABCD", makeScript(), false)
	if err := store.AddPlayer(g.ID, "user-1", "Alice"); err != nil {
		t.Fatalf("AddPlayer: %v", err)
	}
	if !store.HasPlayer(g.ID, "user-1") {
		t.Error("HasPlayer(user-1) = false")
	}
	if store.HasPlayer(g.ID, "user-x") {
		t.Error("HasPlayer(user-x) = true")
	}
	got, _ := store.Get(g.ID)
	if got.Players["user-1"].Name != "Alice" {
		t.Errorf("Name = %q; want Alice", got.Players["user-1"].Name)
	}
}

func TestPostgresStore_RemovePlayer(t *testing.T) {
	store := newPostgresFixture(t)
	g, _ := store.New("ABCD", makeScript(), false)
	_ = store.AddPlayer(g.ID, "user-1", "Alice")
	if err := store.RemovePlayer(g.ID, "user-1"); err != nil {
		t.Fatalf("RemovePlayer: %v", err)
	}
	if store.HasPlayer(g.ID, "user-1") {
		t.Error("HasPlayer(user-1) = true after remove")
	}
}

func TestPostgresStore_ConnectDisconnectPlayer(t *testing.T) {
	store := newPostgresFixture(t)
	g, _ := store.New("ABCD", makeScript(), false)
	_ = store.AddPlayer(g.ID, "user-1", "Alice")
	if err := store.ConnectPlayer(g.ID, "user-1"); err != nil {
		t.Fatalf("ConnectPlayer: %v", err)
	}
	got, _ := store.Get(g.ID)
	if !got.Players["user-1"].Connected {
		t.Errorf("Connected = false after ConnectPlayer")
	}
	if err := store.DisconnectPlayer(g.ID, "user-1"); err != nil {
		t.Fatalf("DisconnectPlayer: %v", err)
	}
	got, _ = store.Get(g.ID)
	if got.Players["user-1"].Connected {
		t.Errorf("Connected = true after DisconnectPlayer")
	}
}
```

- [ ] **Step 5.2 — Run to confirm FAIL**

```bash
INDRI_TEST_POSTGRES_URI="postgres://indri:indri@localhost:5432/indri?sslmode=disable" \
  go test ./internal/repo/game/ -run TestPostgresStore_.*Player -v
```

- [ ] **Step 5.3 — Implement**

Create `internal/repo/game/postgres_player.go`:

```go
package game

import (
	"fmt"

	"github.com/robbiebyrd/indri/internal/models"
	repoErrors "github.com/robbiebyrd/indri/internal/repo"
)

func (s *PostgresStore) AddPlayer(id string, userId string, displayName string) error {
	return s.Mutate(id, func(g *models.Game) error {
		if _, exists := g.Players[userId]; exists {
			return fmt.Errorf("player %q: %w", userId, repoErrors.ErrDuplicate)
		}
		if g.Players == nil {
			g.Players = map[string]models.Player{}
		}
		g.Players[userId] = models.Player{
			Name:      displayName,
			Connected: false,
		}
		return nil
	})
}

func (s *PostgresStore) RemovePlayer(id string, userId string) error {
	return s.Mutate(id, func(g *models.Game) error {
		delete(g.Players, userId)
		return nil
	})
}

func (s *PostgresStore) HasPlayer(id string, userId string) bool {
	g, err := s.Get(id)
	if err != nil {
		return false
	}
	_, has := g.Players[userId]
	return has
}

func (s *PostgresStore) PlayerOnATeam(id string, userId string) bool {
	g, err := s.Get(id)
	if err != nil {
		return false
	}
	for _, team := range g.Teams {
		for _, pid := range team.PlayerIDs {
			if pid == userId {
				return true
			}
		}
	}
	return false
}

func (s *PostgresStore) ConnectPlayer(id string, userId string) error {
	return s.Mutate(id, func(g *models.Game) error {
		p, ok := g.Players[userId]
		if !ok {
			return fmt.Errorf("player %q: %w", userId, repoErrors.ErrNotFound)
		}
		p.Connected = true
		g.Players[userId] = p
		return nil
	})
}

func (s *PostgresStore) DisconnectPlayer(id string, userId string) error {
	return s.Mutate(id, func(g *models.Game) error {
		p, ok := g.Players[userId]
		if !ok {
			return fmt.Errorf("player %q: %w", userId, repoErrors.ErrNotFound)
		}
		p.Connected = false
		g.Players[userId] = p
		return nil
	})
}
```

- [ ] **Step 5.4 — Run tests**

```bash
INDRI_TEST_POSTGRES_URI="postgres://indri:indri@localhost:5432/indri?sslmode=disable" \
  go test ./internal/repo/game/ -run TestPostgresStore -v
```

- [ ] **Step 5.5 — Commit**

```bash
git status
git add internal/repo/game/postgres_player.go internal/repo/game/postgres_test.go
git commit -m "feat(game/postgres): add player operations"
```

---

## Task 6: `game.PostgresStore` — Team operations

`Team.PlayerIDs []string` is a slice of userIDs. There is no `Team.ID` field; team identity is the map key in `Game.Teams`. Operations mirror `memory_team.go` exactly, with `s.Mutate` as the write primitive.

**Files:**
- Create: `internal/repo/game/postgres_team.go`
- Modify: `internal/repo/game/postgres_test.go`

- [ ] **Step 6.1 — Write failing tests**

Append to `internal/repo/game/postgres_test.go`:

```go
func TestPostgresStore_AddPlayerToTeam_ThenHasPlayerOnTeam(t *testing.T) {
	store := newPostgresFixture(t)
	g, _ := store.New("ABCD", scriptWithTeams(), false)
	_ = store.AddPlayer(g.ID, "user-1", "Alice")
	if err := store.AddPlayerToTeam(g.ID, "red", "user-1"); err != nil {
		t.Fatalf("AddPlayerToTeam: %v", err)
	}
	if !store.HasPlayerOnTeam(g.ID, "red", "user-1") {
		t.Fatal("HasPlayerOnTeam(red, user-1) = false")
	}
}

func TestPostgresStore_ChangePlayerTeam(t *testing.T) {
	store := newPostgresFixture(t)
	g, _ := store.New("ABCD", scriptWithTeams(), false)
	_ = store.AddPlayer(g.ID, "user-1", "Alice")
	_ = store.AddPlayerToTeam(g.ID, "red", "user-1")
	if err := store.ChangePlayerTeam(g.ID, "blue", "user-1"); err != nil {
		t.Fatalf("ChangePlayerTeam: %v", err)
	}
	teamID, err := store.PlayerOnWhichTeam(g.ID, "user-1")
	if err != nil {
		t.Fatalf("PlayerOnWhichTeam: %v", err)
	}
	if teamID == nil || *teamID != "blue" {
		t.Errorf("team: want blue, got %v", teamID)
	}
}

func TestPostgresStore_RemovePlayerFromTeam(t *testing.T) {
	store := newPostgresFixture(t)
	g, _ := store.New("ABCD", scriptWithTeams(), false)
	_ = store.AddPlayer(g.ID, "user-1", "Alice")
	_ = store.AddPlayerToTeam(g.ID, "red", "user-1")
	if err := store.RemovePlayerFromTeam(g.ID, "user-1"); err != nil {
		t.Fatalf("RemovePlayerFromTeam: %v", err)
	}
	if store.HasPlayerOnTeam(g.ID, "red", "user-1") {
		t.Errorf("still on team after remove")
	}
}
```

- [ ] **Step 6.2 — Run to confirm FAIL**

- [ ] **Step 6.3 — Implement**

Create `internal/repo/game/postgres_team.go`:

```go
package game

import (
	"fmt"

	"github.com/robbiebyrd/indri/internal/models"
	repoErrors "github.com/robbiebyrd/indri/internal/repo"
)

func (s *PostgresStore) HasPlayerOnTeam(id string, teamId string, userId string) bool {
	g, err := s.Get(id)
	if err != nil {
		return false
	}
	team, ok := g.Teams[teamId]
	if !ok {
		return false
	}
	return containsID(team.PlayerIDs, userId)
}

func (s *PostgresStore) AddPlayerToTeam(id string, teamId string, userId string) error {
	return s.Mutate(id, func(g *models.Game) error {
		team, ok := g.Teams[teamId]
		if !ok {
			return fmt.Errorf("team %q: %w", teamId, repoErrors.ErrNotFound)
		}
		if _, ok := g.Players[userId]; !ok {
			return fmt.Errorf("player %q: %w", userId, repoErrors.ErrNotFound)
		}
		if !containsID(team.PlayerIDs, userId) {
			team.PlayerIDs = append(team.PlayerIDs, userId)
			g.Teams[teamId] = team
		}
		return nil
	})
}

func (s *PostgresStore) RemovePlayerFromTeam(id string, userId string) error {
	return s.Mutate(id, func(g *models.Game) error {
		for tid, team := range g.Teams {
			if containsID(team.PlayerIDs, userId) {
				team.PlayerIDs = filterOutID(team.PlayerIDs, userId)
				g.Teams[tid] = team
			}
		}
		return nil
	})
}

func (s *PostgresStore) ChangePlayerTeam(id string, teamId string, userId string) error {
	return s.Mutate(id, func(g *models.Game) error {
		newTeam, ok := g.Teams[teamId]
		if !ok {
			return fmt.Errorf("team %q: %w", teamId, repoErrors.ErrNotFound)
		}
		if _, ok := g.Players[userId]; !ok {
			return fmt.Errorf("player %q: %w", userId, repoErrors.ErrNotFound)
		}
		for tid, team := range g.Teams {
			if containsID(team.PlayerIDs, userId) {
				team.PlayerIDs = filterOutID(team.PlayerIDs, userId)
				g.Teams[tid] = team
			}
		}
		newTeam.PlayerIDs = append(newTeam.PlayerIDs, userId)
		g.Teams[teamId] = newTeam
		return nil
	})
}

func (s *PostgresStore) PlayerOnWhichTeam(id string, userId string) (*string, error) {
	g, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	for tid, team := range g.Teams {
		if containsID(team.PlayerIDs, userId) {
			out := tid
			return &out, nil
		}
	}
	return nil, nil
}
```

Note: `containsID` and `filterOutID` are already defined in `memory_team.go` in the same package. Do not redefine them.

- [ ] **Step 6.4 — Run tests**

```bash
INDRI_TEST_POSTGRES_URI="postgres://indri:indri@localhost:5432/indri?sslmode=disable" \
  go test ./internal/repo/game/ -run TestPostgresStore -v
```

- [ ] **Step 6.5 — Commit**

```bash
git status
git add internal/repo/game/postgres_team.go internal/repo/game/postgres_test.go
git commit -m "feat(game/postgres): add team operations"
```

---

## Task 7: `game.PostgresStore` — Host operations + restore Storer assertion

**Files:**
- Create: `internal/repo/game/postgres_host.go`
- Modify: `internal/repo/game/postgres.go` (uncomment assertion)
- Modify: `internal/repo/game/postgres_test.go`

- [ ] **Step 7.1 — Write failing tests**

Append to `internal/repo/game/postgres_test.go`:

```go
func TestPostgresStore_SetPlayerAsHost_ThenPlayerIsHost(t *testing.T) {
	store := newPostgresFixture(t)
	g, _ := store.New("ABCD", makeScript(), false)
	_ = store.AddPlayer(g.ID, "user-1", "Alice")
	if store.HasHost(g.ID) {
		t.Fatal("HasHost = true before SetPlayerAsHost")
	}
	if err := store.SetPlayerAsHost(g.ID, "user-1"); err != nil {
		t.Fatalf("SetPlayerAsHost: %v", err)
	}
	if !store.HasHost(g.ID) || !store.PlayerIsHost(g.ID, "user-1") {
		t.Error("SetPlayerAsHost did not take effect")
	}
}

func TestPostgresStore_UnsetHost(t *testing.T) {
	store := newPostgresFixture(t)
	g, _ := store.New("ABCD", makeScript(), false)
	_ = store.AddPlayer(g.ID, "user-1", "Alice")
	_ = store.SetPlayerAsHost(g.ID, "user-1")
	if err := store.UnsetHost(g.ID); err != nil {
		t.Fatalf("UnsetHost: %v", err)
	}
	if store.HasHost(g.ID) {
		t.Error("HasHost = true after UnsetHost")
	}
}
```

- [ ] **Step 7.2 — Run to confirm FAIL**

- [ ] **Step 7.3 — Implement**

Create `internal/repo/game/postgres_host.go`:

```go
package game

import (
	"fmt"

	"github.com/robbiebyrd/indri/internal/models"
	repoErrors "github.com/robbiebyrd/indri/internal/repo"
)

func (s *PostgresStore) HasHost(id string) bool {
	g, err := s.Get(id)
	if err != nil {
		return false
	}
	for _, p := range g.Players {
		if p.Host {
			return true
		}
	}
	return false
}

func (s *PostgresStore) PlayerIsHost(id string, playerId string) bool {
	g, err := s.Get(id)
	if err != nil {
		return false
	}
	p, ok := g.Players[playerId]
	return ok && p.Host
}

func (s *PostgresStore) SetPlayerAsHost(id string, playerId string) error {
	return s.Mutate(id, func(g *models.Game) error {
		if _, ok := g.Players[playerId]; !ok {
			return fmt.Errorf("player %q: %w", playerId, repoErrors.ErrNotFound)
		}
		for uid, p := range g.Players {
			p.Host = (uid == playerId)
			g.Players[uid] = p
		}
		return nil
	})
}

func (s *PostgresStore) UnsetHost(id string) error {
	return s.Mutate(id, func(g *models.Game) error {
		for uid, p := range g.Players {
			p.Host = false
			g.Players[uid] = p
		}
		return nil
	})
}
```

- [ ] **Step 7.4 — Restore the compile-time assertion**

In `internal/repo/game/postgres.go`, replace the commented-out placeholder with:

```go
var _ Storer = (*PostgresStore)(nil)
```

- [ ] **Step 7.5 — Build + test**

```bash
go build ./...
INDRI_TEST_POSTGRES_URI="postgres://indri:indri@localhost:5432/indri?sslmode=disable" \
  go test ./internal/repo/game/ -v
```

Expected: PASS. The assertion confirms the interface is fully satisfied.

- [ ] **Step 7.6 — Commit**

```bash
git status
git add internal/repo/game/postgres_host.go internal/repo/game/postgres.go internal/repo/game/postgres_test.go
git commit -m "feat(game/postgres): add host operations; complete Storer implementation"
```

---

## Task 8: `user.PostgresStore` and `session.PostgresStore`

### User DDL

```sql
CREATE TABLE IF NOT EXISTS users (
    id    TEXT PRIMARY KEY,
    email TEXT UNIQUE NOT NULL,
    data  JSONB NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_users_email ON users (email);
```

### Session DDL

```sql
CREATE TABLE IF NOT EXISTS sessions (
    id      TEXT PRIMARY KEY,
    token   TEXT UNIQUE,
    user_id TEXT UNIQUE,
    data    JSONB NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_token   ON sessions (token);
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions (user_id);
```

`user_id` has a UNIQUE index to enforce one-session-per-user, mirroring MongoStore's unique index on `userId`.

**Files:**
- Create: `internal/repo/user/postgres.go`
- Create: `internal/repo/user/postgres_test.go`
- Create: `internal/repo/session/postgres.go`
- Create: `internal/repo/session/postgres_test.go`

- [ ] **Step 8.1 — Write failing user tests**

Create `internal/repo/user/postgres_test.go`:

```go
package user

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/robbiebyrd/indri/internal/models"
	repoErrors "github.com/robbiebyrd/indri/internal/repo"
)

func postgresUserURI(t *testing.T) string {
	t.Helper()
	uri := os.Getenv("INDRI_TEST_POSTGRES_URI")
	if uri == "" {
		t.Skip("INDRI_TEST_POSTGRES_URI not set; skipping Postgres integration test")
	}
	return uri
}

func newPostgresUserFixture(t *testing.T) *PostgresStore {
	t.Helper()
	store, err := NewPostgresStore(context.Background(), postgresUserURI(t))
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}
	t.Cleanup(func() { store.db.Exec("TRUNCATE TABLE users") })
	return store
}

func TestUserPostgresStore_New_AssignsID(t *testing.T) {
	s := newPostgresUserFixture(t)
	u, err := s.New(models.CreateUser{Email: "a@b.c", Name: "Alice"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if u.ID == "" {
		t.Fatal("ID empty")
	}
}

func TestUserPostgresStore_New_DuplicateEmail(t *testing.T) {
	s := newPostgresUserFixture(t)
	_, _ = s.New(models.CreateUser{Email: "a@b.c", Name: "Alice"})
	_, err := s.New(models.CreateUser{Email: "a@b.c", Name: "Alice2"})
	if !errors.Is(err, repoErrors.ErrDuplicate) {
		t.Fatalf("want ErrDuplicate, got %v", err)
	}
}

func TestUserPostgresStore_Get_UnknownID(t *testing.T) {
	s := newPostgresUserFixture(t)
	_, err := s.Get("nope")
	if !errors.Is(err, repoErrors.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestUserPostgresStore_FindFirst(t *testing.T) {
	s := newPostgresUserFixture(t)
	u, _ := s.New(models.CreateUser{Email: "a@b.c", Name: "Alice"})
	got, err := s.FindFirst("email", "a@b.c")
	if err != nil || got.ID != u.ID {
		t.Fatalf("FindFirst: got=%v err=%v", got, err)
	}
}
```

- [ ] **Step 8.2 — Implement `user.PostgresStore`**

Create `internal/repo/user/postgres.go`:

```go
package user

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/robbiebyrd/indri/internal/models"
	repoErrors "github.com/robbiebyrd/indri/internal/repo"
	"github.com/robbiebyrd/indri/internal/repo/ids"
)

const userSchemaSQL = `
CREATE TABLE IF NOT EXISTS users (
    id    TEXT PRIMARY KEY,
    email TEXT UNIQUE NOT NULL,
    data  JSONB NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_users_email ON users (email);
`

type PostgresStore struct {
	ctx context.Context
	db  *sql.DB
}

var _ Storer = (*PostgresStore)(nil)

func NewPostgresStore(ctx context.Context, uri string) (*PostgresStore, error) {
	if uri == "" {
		return nil, errors.New("postgres URI is required")
	}
	db, err := sql.Open("pgx", uri)
	if err != nil {
		return nil, fmt.Errorf("opening postgres connection: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("pinging postgres: %w", err)
	}
	if _, err := db.ExecContext(ctx, userSchemaSQL); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("running user schema DDL: %w", err)
	}
	return &PostgresStore{ctx: ctx, db: db}, nil
}

func isPgDuplicateKey(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "23505") ||
		strings.Contains(err.Error(), "duplicate key value violates unique constraint")
}

func (s *PostgresStore) New(c models.CreateUser) (*models.User, error) {
	now := time.Now()
	u := &models.User{
		ID:          ids.New(),
		Email:       c.Email,
		Name:        c.Name,
		DisplayName: c.DisplayName,
		Password:    c.Password,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	data, err := json.Marshal(u)
	if err != nil {
		return nil, fmt.Errorf("marshaling user: %w", err)
	}
	_, err = s.db.ExecContext(s.ctx,
		`INSERT INTO users (id, email, data) VALUES ($1, $2, $3)`,
		u.ID, u.Email, data,
	)
	if err != nil {
		if isPgDuplicateKey(err) {
			return nil, fmt.Errorf("email %q: %w", c.Email, repoErrors.ErrDuplicate)
		}
		return nil, fmt.Errorf("inserting user: %w", err)
	}
	return s.Get(u.ID)
}

func (s *PostgresStore) Get(id string) (*models.User, error) {
	var data []byte
	err := s.db.QueryRowContext(s.ctx,
		`SELECT data FROM users WHERE id = $1`, id,
	).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("id %q: %w", id, repoErrors.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("querying user: %w", err)
	}
	var u models.User
	if err := json.Unmarshal(data, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *PostgresStore) Exists(id string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(s.ctx,
		`SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)`, id,
	).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

func (s *PostgresStore) Find(key string, value string) ([]*models.User, error) {
	var col string
	switch key {
	case "email":
		col = "email"
	default:
		// Generic fallback: scan all and filter in Go. Only email is indexed.
		return s.findInGo(key, value)
	}
	rows, err := s.db.QueryContext(s.ctx,
		fmt.Sprintf(`SELECT data FROM users WHERE %s = $1`, col), value,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanUsers(rows)
}

func (s *PostgresStore) findInGo(key, value string) ([]*models.User, error) {
	rows, err := s.db.QueryContext(s.ctx, `SELECT data FROM users`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	all, err := scanUsers(rows)
	if err != nil {
		return nil, err
	}
	var out []*models.User
	for _, u := range all {
		if matchUserField(u, key, value) {
			out = append(out, u)
		}
	}
	return out, nil
}

func (s *PostgresStore) FindFirst(key string, value string) (*models.User, error) {
	list, err := s.Find(key, value)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("no user with %s=%q: %w", key, value, repoErrors.ErrNotFound)
	}
	return list[0], nil
}

func (s *PostgresStore) Update(u *models.UpdateUser) error {
	if u == nil || u.ID == "" {
		return errors.New("UpdateUser.ID is required")
	}
	existing, err := s.Get(u.ID)
	if err != nil {
		return err
	}
	if u.Email != "" {
		existing.Email = u.Email
	}
	if u.Name != "" {
		existing.Name = u.Name
	}
	if u.DisplayName != nil {
		existing.DisplayName = u.DisplayName
	}
	if u.Password != nil {
		existing.Password = u.Password
	}
	existing.UpdatedAt = time.Now()

	data, err := json.Marshal(existing)
	if err != nil {
		return fmt.Errorf("marshaling user: %w", err)
	}
	_, err = s.db.ExecContext(s.ctx,
		`UPDATE users SET email = $2, data = $3 WHERE id = $1`,
		existing.ID, existing.Email, data,
	)
	if err != nil {
		if isPgDuplicateKey(err) {
			return fmt.Errorf("email %q: %w", existing.Email, repoErrors.ErrDuplicate)
		}
		return fmt.Errorf("updating user: %w", err)
	}
	return nil
}

func scanUsers(rows *sql.Rows) ([]*models.User, error) {
	var out []*models.User
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var u models.User
		if err := json.Unmarshal(data, &u); err != nil {
			return nil, err
		}
		out = append(out, &u)
	}
	return out, rows.Err()
}
```

Note: `matchUserField` is already defined in `user/memory.go` (same package), so do NOT redeclare it here. Its existing signature and semantics work identically for the Postgres store.

- [ ] **Step 8.3 — Run user tests**

```bash
INDRI_TEST_POSTGRES_URI="postgres://indri:indri@localhost:5432/indri?sslmode=disable" \
  go test ./internal/repo/user/ -v
```

- [ ] **Step 8.4 — Write failing session tests**

Create `internal/repo/session/postgres_test.go`:

```go
package session

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/robbiebyrd/indri/internal/models"
	repoErrors "github.com/robbiebyrd/indri/internal/repo"
)

func postgresSessionURI(t *testing.T) string {
	t.Helper()
	uri := os.Getenv("INDRI_TEST_POSTGRES_URI")
	if uri == "" {
		t.Skip("INDRI_TEST_POSTGRES_URI not set; skipping Postgres integration test")
	}
	return uri
}

func newPostgresSessionFixture(t *testing.T) *PostgresStore {
	t.Helper()
	store, err := NewPostgresStore(context.Background(), postgresSessionURI(t))
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}
	t.Cleanup(func() { store.db.Exec("TRUNCATE TABLE sessions") })
	return store
}

func TestSessionPostgresStore_New_AssignsIDAndKeepsToken(t *testing.T) {
	s := newPostgresSessionFixture(t)
	sess, err := s.New(models.CreateSession{Token: "tok-1", UserID: "u-1"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if sess.ID == "" || sess.Token != "tok-1" {
		t.Errorf("bad session: %+v", sess)
	}
}

func TestSessionPostgresStore_New_DuplicateUserID_ReturnsExisting(t *testing.T) {
	s := newPostgresSessionFixture(t)
	first, _ := s.New(models.CreateSession{Token: "t-1", UserID: "u-1"})
	again, err := s.New(models.CreateSession{Token: "t-2", UserID: "u-1"})
	if err != nil {
		t.Fatalf("New (dup user): %v", err)
	}
	if again.ID != first.ID {
		t.Errorf("expected same session ID; got %q vs %q", again.ID, first.ID)
	}
}

func TestSessionPostgresStore_GetByToken(t *testing.T) {
	s := newPostgresSessionFixture(t)
	first, _ := s.New(models.CreateSession{Token: "t-1", UserID: "u-1"})
	got, err := s.GetByToken("t-1")
	if err != nil || got.ID != first.ID {
		t.Fatalf("GetByToken: got=%v err=%v", got, err)
	}
}

func TestSessionPostgresStore_Delete(t *testing.T) {
	s := newPostgresSessionFixture(t)
	sess, _ := s.New(models.CreateSession{Token: "t-1", UserID: "u-1"})
	if err := s.Delete(sess.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := s.Get(sess.ID)
	if !errors.Is(err, repoErrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound after Delete, got %v", err)
	}
}
```

- [ ] **Step 8.5 — Implement `session.PostgresStore`**

Create `internal/repo/session/postgres.go`:

```go
package session

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/robbiebyrd/indri/internal/models"
	repoErrors "github.com/robbiebyrd/indri/internal/repo"
	"github.com/robbiebyrd/indri/internal/repo/ids"
)

const sessionSchemaSQL = `
CREATE TABLE IF NOT EXISTS sessions (
    id      TEXT PRIMARY KEY,
    token   TEXT UNIQUE,
    user_id TEXT UNIQUE,
    data    JSONB NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_token   ON sessions (token);
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions (user_id);
`

type PostgresStore struct {
	ctx context.Context
	db  *sql.DB
}

var _ Storer = (*PostgresStore)(nil)

func NewPostgresStore(ctx context.Context, uri string) (*PostgresStore, error) {
	if uri == "" {
		return nil, errors.New("postgres URI is required")
	}
	db, err := sql.Open("pgx", uri)
	if err != nil {
		return nil, fmt.Errorf("opening postgres connection: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("pinging postgres: %w", err)
	}
	if _, err := db.ExecContext(ctx, sessionSchemaSQL); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("running session schema DDL: %w", err)
	}
	return &PostgresStore{ctx: ctx, db: db}, nil
}

func isPgDuplicateKey(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "23505") ||
		strings.Contains(err.Error(), "duplicate key value violates unique constraint")
}

func ptrToStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func strToPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// New enforces one-session-per-userId: a second call for the same UserID
// returns the existing session, matching MongoStore behaviour.
func (s *PostgresStore) New(c models.CreateSession) (*models.Session, error) {
	if c.UserID == "" {
		return nil, errors.New("session must have a user ID")
	}

	// Return existing session for this user if one exists.
	existing, err := s.FindFirst("userId", c.UserID)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, repoErrors.ErrNotFound) {
		return nil, err
	}

	sess := &models.Session{
		ID:        ids.New(),
		Token:     c.Token,
		UserID:    strToPtr(c.UserID),
		GameID:    strToPtr(c.GameID),
		TeamID:    strToPtr(c.TeamID),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	data, err := json.Marshal(sess)
	if err != nil {
		return nil, fmt.Errorf("marshaling session: %w", err)
	}
	_, err = s.db.ExecContext(s.ctx,
		`INSERT INTO sessions (id, token, user_id, data) VALUES ($1, $2, $3, $4)`,
		sess.ID, sess.Token, c.UserID, data,
	)
	if err != nil {
		if isPgDuplicateKey(err) {
			// Race: another writer created the session between our FindFirst
			// check and now. Re-read and return it.
			return s.FindFirst("userId", c.UserID)
		}
		return nil, fmt.Errorf("inserting session: %w", err)
	}
	return s.Get(sess.ID)
}

func (s *PostgresStore) Get(id string) (*models.Session, error) {
	var data []byte
	err := s.db.QueryRowContext(s.ctx,
		`SELECT data FROM sessions WHERE id = $1`, id,
	).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("id %q: %w", id, repoErrors.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("querying session: %w", err)
	}
	var sess models.Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, err
	}
	return &sess, nil
}

func (s *PostgresStore) GetByToken(token string) (*models.Session, error) {
	if token == "" {
		return nil, errors.New("token is empty")
	}
	var data []byte
	err := s.db.QueryRowContext(s.ctx,
		`SELECT data FROM sessions WHERE token = $1`, token,
	).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("token: %w", repoErrors.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("querying session by token: %w", err)
	}
	var sess models.Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, err
	}
	return &sess, nil
}

func (s *PostgresStore) Exists(id string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(s.ctx,
		`SELECT EXISTS(SELECT 1 FROM sessions WHERE id = $1)`, id,
	).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

func (s *PostgresStore) Find(key string, value string) ([]*models.Session, error) {
	var col string
	switch key {
	case "token":
		col = "token"
	case "userId":
		col = "user_id"
	default:
		return s.findInGo(key, value)
	}
	rows, err := s.db.QueryContext(s.ctx,
		fmt.Sprintf(`SELECT data FROM sessions WHERE %s = $1`, col), value,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSessions(rows)
}

func (s *PostgresStore) findInGo(key, value string) ([]*models.Session, error) {
	rows, err := s.db.QueryContext(s.ctx, `SELECT data FROM sessions`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	all, err := scanSessions(rows)
	if err != nil {
		return nil, err
	}
	var out []*models.Session
	for _, sess := range all {
		if matchSessionField(sess, key, value) {
			out = append(out, sess)
		}
	}
	return out, nil
}

func (s *PostgresStore) FindFirst(key string, value string) (*models.Session, error) {
	list, err := s.Find(key, value)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("%s=%q: %w", key, value, repoErrors.ErrNotFound)
	}
	return list[0], nil
}

func (s *PostgresStore) Update(id string, u *models.UpdateSession) error {
	if u == nil {
		return errors.New("UpdateSession is nil")
	}
	sess, err := s.Get(id)
	if err != nil {
		return err
	}
	if u.GameID != "" {
		sess.GameID = strToPtr(u.GameID)
	}
	if u.UserID != "" {
		sess.UserID = strToPtr(u.UserID)
	}
	if u.TeamID != "" {
		sess.TeamID = strToPtr(u.TeamID)
	}
	sess.UpdatedAt = time.Now()

	data, err := json.Marshal(sess)
	if err != nil {
		return fmt.Errorf("marshaling session: %w", err)
	}
	userID := ""
	if sess.UserID != nil {
		userID = *sess.UserID
	}
	_, err = s.db.ExecContext(s.ctx,
		`UPDATE sessions SET user_id = $2, data = $3 WHERE id = $1`,
		sess.ID, userID, data,
	)
	return err
}

func (s *PostgresStore) Delete(id string) error {
	_, err := s.db.ExecContext(s.ctx, `DELETE FROM sessions WHERE id = $1`, id)
	return err
}

func scanSessions(rows *sql.Rows) ([]*models.Session, error) {
	var out []*models.Session
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var sess models.Session
		if err := json.Unmarshal(data, &sess); err != nil {
			return nil, err
		}
		out = append(out, &sess)
	}
	return out, rows.Err()
}
```

Note: `matchSessionField` is already defined in `session/memory.go` (same package), so do NOT redeclare it here.

- [ ] **Step 8.6 — Run session tests**

```bash
INDRI_TEST_POSTGRES_URI="postgres://indri:indri@localhost:5432/indri?sslmode=disable" \
  go test ./internal/repo/session/ -v
```

- [ ] **Step 8.7 — Full build**

```bash
go build ./...
```

- [ ] **Step 8.8 — Commit**

```bash
git status
git add internal/repo/user/postgres.go internal/repo/user/postgres_test.go \
        internal/repo/session/postgres.go internal/repo/session/postgres_test.go
git commit -m "feat(repo): add PostgresStore for user and session"
```

---

## Task 9: Wire `"postgres"` into the injector + end-to-end smoke test

**Files:**
- Modify: `internal/injector/repos.go`

- [ ] **Step 9.1 — Add the `"postgres"` case**

In `internal/injector/repos.go`, add after the `"memory"` case:

```go
case "postgres":
    gr, err = gameRepo.NewPostgresStore(ctx, env.PostgresURI, clients.LockManager, clients.Publisher)
    if err != nil {
        return nil, err
    }
    ur, err = userRepo.NewPostgresStore(ctx, env.PostgresURI)
    if err != nil {
        return nil, err
    }
    sr, err = sessionRepo.NewPostgresStore(ctx, env.PostgresURI)
    if err != nil {
        return nil, err
    }
```

Add imports as needed:

```go
gameRepo    "github.com/robbiebyrd/indri/internal/repo/game"
userRepo    "github.com/robbiebyrd/indri/internal/repo/user"
sessionRepo "github.com/robbiebyrd/indri/internal/repo/session"
```

These imports are already present; verify before adding.

- [ ] **Step 9.2 — Build + full test suite**

```bash
go build ./...
go test ./...
```

Expected: PASS. Postgres tests skip when `INDRI_TEST_POSTGRES_URI` is unset.

- [ ] **Step 9.3 — Smoke test: server starts on postgres backend**

With Postgres running (see Docker one-liner at top):

```bash
INDRI_DB_BACKEND=postgres \
INDRI_POSTGRES_URI="postgres://indri:indri@localhost:5432/indri?sslmode=disable" \
  go run ./cmd/server -script ./example/server.json > /tmp/indri-pg.log 2>&1 &
SERVER_PID=$!
sleep 2
nc -z localhost 5002 && echo "listening" || echo "not listening"
grep -i "error\|panic\|fatal" /tmp/indri-pg.log || echo "no errors in log"
kill $SERVER_PID 2>/dev/null
wait 2>/dev/null
```

Expected: `listening`, then `no errors in log`.

- [ ] **Step 9.4 — Verify unknown backend still fails fast**

```bash
INDRI_DB_BACKEND=nope go run ./cmd/server -script ./example/server.json 2>&1 | head -5
```

Expected: exits with `unknown database backend "nope"`.

- [ ] **Step 9.5 — Commit**

```bash
git status
git add internal/injector/repos.go
git commit -m "feat(injector): wire postgres backend case"
```

---

## Final Verification

```bash
go vet ./...
go build ./...
go test ./...
go test -race ./...

# With Postgres reachable:
INDRI_TEST_POSTGRES_URI="postgres://indri:indri@localhost:5432/indri?sslmode=disable" \
  go test -race ./...

# With MongoDB reachable (confirm existing backend is unbroken):
INDRI_TEST_MONGO_URI="mongodb://localhost:27017/?directConnection=true" \
  go test -race ./...
```

Expected: all PASS. Postgres integration tests run only when `INDRI_TEST_POSTGRES_URI` is set; MongoDB integration tests run only when `INDRI_TEST_MONGO_URI` or `INDRI_MONGO_URI` is set. In-memory and unit tests run unconditionally.

Manual end-to-end:

```bash
INDRI_DB_BACKEND=postgres \
INDRI_POSTGRES_URI="postgres://indri:indri@localhost:5432/indri?sslmode=disable" \
  go run ./example/tictactoe -script ./config.json
```

Connect a client, play a full join → move → end. Restart the server and confirm game state persists (unlike in-memory).

---

## Design decisions (summary)

| Decision | Choice | Rationale |
|---|---|---|
| SQL driver | `github.com/jackc/pgx/v5/stdlib` | Actively maintained; `stdlib` adapter keeps code using standard `database/sql` API without pgx-specific types leaking into store files |
| Schema shape | JSONB blob + indexed scalar columns | Minimal code surface; parity with MongoDB/Memory write paths; JSONB is fast and indexable |
| JSON encoding | `encoding/json` | No extra dependency; consistent with MemoryStore |
| Mutate version fence | `UPDATE ... WHERE id=$1 AND version=$2`, check `RowsAffected` | Simpler than `SELECT FOR UPDATE`; single-statement atomic in Postgres; matches MemoryStore pattern |
| UpdateField / DeleteField | Read-modify-write full JSON in Go | Keeps delta-computation path identical to all other backends; avoids `jsonb_set` SQL expressions that would duplicate `applyDottedPath` logic |
| Test guard | `INDRI_TEST_POSTGRES_URI` env var | Consistent with `INDRI_TEST_MONGO_URI`; skip automatically in CI unless Postgres is provisioned |

---

## Open Questions

None. All design decisions are resolved above.
