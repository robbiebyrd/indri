# Story 082 — user and session backends, and choosing a backend at runtime

Reference: `main`'s `internal/repo/{user,session}`, re-shaped to local's pattern rather than copied.

## Shape

Per repo (`user`, `session`): the existing `Storer`, and four implementations.

- `Store` — MongoDB (local name kept). Maps the driver onto `repo.ErrNotFound` / `repo.ErrDuplicate`.
- `MemoryStore` — maps under one RWMutex, hands out copies.
- `SQLStore` — **one** implementation for SQLite and PostgreSQL (`NewSQLiteStore`, `NewPostgresStore`),
  as `repo/game` does with `sqlDocs`. Statements are written once with `?` and rebound per dialect.
- The SQL dialect (`placeholder`, `rebind`, `isUniqueViolation`) moves out of `repo/game` into a shared
  package `internal/repo/sqldialect`, so game, user and session use one copy.

Shared per package (`fields.go`): building a record from its create model, applying an update's set
fields, matching a lookup key, and the set of lookup keys a store accepts. An unknown key is an error
on every backend, never a silent empty result.

## Behaviours (contract-tested on all four backends; no skips beyond "database unreachable")

User
- `New` mints the id; a taken email is `repo.ErrDuplicate`.
- `Get` of a missing id is `repo.ErrNotFound`; so is `FindFirst` with no match.
- `Update` changes only the fields it is given — the password survives an update that omits it (284d617);
  a missing id is `repo.ErrNotFound`; moving onto a taken email is `repo.ErrDuplicate`.
- Password is not in the JSON form (`json:"-"`), so SQL keeps it in its own column.

Session
- One session per user: `New` returns the user's existing session. The `userId` unique index stays.
- `New` keeps every field, `SlotID` included; `Update` sets only non-empty fields (191813c).
- `Update` of a missing or deleted session is `repo.ErrNotFound` (f5870ec); moving a session onto a user
  who has one is `repo.ErrDuplicate` (33bf874).
- A session older than `sessionMaxAge` is gone on every read and `New` gives its user a fresh one
  (2527068, 6b8bdd4). On MongoDB the reads filter on `createdAt` too, so a token is dead at the cutoff,
  not up to a TTL-sweep later — and the test runs there instead of skipping.
- Token is `json:"-"`, so SQL keeps it in its own column. `created_at` is Unix nanoseconds on both SQL
  dialects (SQLite's existing column; PostgreSQL's table is new, so it matches rather than diverges).
- `LeaveGame` is story 089, not here.

Both
- Client-facing create models carry no id: `register` decodes the payload straight into `CreateUser`,
  and a client-chosen id could replace another account (review finding). Stores always mint.
- SQL `Update` reads and writes in one transaction with the row locked (`sqldialect.LockForUpdate`:
  `FOR UPDATE` on PostgreSQL; SQLite's single connection already serialises). The game store's
  `mutateStored` had the same lost-update gap on PostgreSQL and takes the same lock.
- Sessions lift `game_id` into an indexed column: every broadcast looks sessions up by game.

## Wiring

- Services and the injector take `Storer` interfaces, not `*Store` (game included, for the switch).
- `INDRI_DB_BACKEND` = `mongodb` (default) | `memory` | `sqlite` | `postgres`, with `INDRI_SQLITE_PATH`
  and `INDRI_POSTGRES_URI`; game, user and session all follow it. Documented in `.env.example`.
- Timers: on sqlite/postgres the schedule store is in memory, with a boot warning (Boss); SQL schedule
  store is story 091. `memory`/`sqlite` with `INDRI_LOCK_BACKEND=redis` is refused at boot: their data
  cannot be shared between instances.
