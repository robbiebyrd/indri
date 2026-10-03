---
id: 082-b854
title: Port user and session backends across memory, SQLite and Postgres
status: complete
priority: P1
type: feature
created: "2026-10-01T13:52:29.872Z"
updated: "2026-10-03T05:31:02.000Z"
dependencies: ["081-7e0a"]
plan: plans/main-divergence-integration.md
plan_step: Step 5
depends_on: ["stories/081-7e0a-complete-P1-port-the-player-slot-model-and-migrate-the-user-ke.md"]
---

# Port user and session backends across memory, SQLite and Postgres

## Problem Statement

Local has Mongo-only user and session stores. origin/main added Storer interfaces plus three backends for each, and then had to fix five real behaviours after shipping them. Porting the stores without those fixes would reintroduce bugs that are already solved upstream.

## Acceptance Criteria

- [x] user and session declare Storer interfaces with memory, sqlite and postgres implementations alongside mongo
- [x] store_contract_test.go for both repos runs against all four backends
- [x] Sessions expire after sessionMaxAge on postgres, sqlite and memory (origin 2527068, 6b8bdd4)
- [x] Update returns ErrNotFound when no row was updated (origin f5870ec)
- [x] Moving a session onto a taken user reports repo.ErrDuplicate (origin 33bf874)
- [x] A user's password survives an Update that omits it (origin 284d617)
- [x] Sessions remain one-per-user; the unique index on userId is preserved on every backend
- [x] Every session store keeps SlotID in New and Update, and Update leaves empty fields alone rather than blanking them (origin 191813c)
- [x] INDRI_DB_BACKEND (mongodb|memory|sqlite|postgres) selects one backend for game, user and session in GetRepos, with INDRI_SQLITE_PATH / INDRI_POSTGRES_URI documented in .env.example (Boss: folded into 082)
- [x] VERIFY: go test ./internal/repo/user/ ./internal/repo/session/

## Files

- internal/repo/user/
- internal/repo/session/

## Proof

- [x] [completeness] Completeness
- [x] [feature-availability] Feature availability
- [x] [robustness] Robustness
- [x] [resilience] Resilience
- [x] [security] Security
- [x] [defense-in-depth] Defense in depth
- [x] [input-validation] Input validation
- [x] [thread-safety] Thread safety
- [x] [configurability] Configurability

## Work Log


### 2026-10-03T05:20:29.000Z - SCOPE. Boss folded the runtime backend switch into 082 (INDRI_DB_BACKEND). Boss chose: on sqlite/postgres, script timers stay in memory with a boot warning; a SQL schedule store is filed as 091. LeaveGame stays in 089.

### 2026-10-03T05:20:29.000Z - IMPLEMENTATION. Re-shaped rather than copied from main: one SQLStore per package serves both SQL dialects (main had near-duplicate sqlite.go/postgres.go and three copies of the update logic). The dialect moved out of repo/game into internal/repo/sqldialect, shared by game, user and session. Per-package fields.go holds create/update/lookup rules once; unknown lookup keys are an error on every backend (session lookups exclude the token, which only GetByToken reads). Mongo stores now map driver errors onto repo.ErrNotFound/ErrDuplicate, and Mongo session reads filter on createdAt so a token dies at sessionMaxAge rather than at the next TTL sweep -- which also lets the expiry test run on Mongo instead of skipping as it does on main. Postgres sessions.created_at is BIGINT Unix nanoseconds, matching SQLite, so one statement set serves both (main used TIMESTAMPTZ). Test fixtures for all three repo packages moved to internal/repo/repotest; the Postgres fixture now selects its schema in the connection string, since the old SET search_path reached only one pooled connection (no leaked rows were found in public, but a public.games table existed for a stray connection to fall into). Services and the injector take Storer interfaces; GetRepos goes through openStores; clients open only the configured database; serve.go closes the SQL pool. .env.example, CLAUDE.md and ARCHITECTURE.md document it.

### 2026-10-03T05:20:29.000Z - VERIFICATION. go build/vet/gofmt clean; go test -race -count=1 ./... green against MongoDB and PostgreSQL, 0 skips. Mutation checks killed: user -- SQL duplicate mapping, update blanking the password, Mongo not-found mapping, SQL dropping the password column, memory email-move check, unknown lookup key, memory duplicate email; session -- Mongo reads ignoring expiry, Mongo New not purging, Mongo update blanking fields, SQL reads ignoring expiry, SQL New not purging, SQL losing the token, SQL duplicate mapping, applyUpdate blanking, alive() always true, memory update duplicate check, unknown lookup key. Survived, by nature: SQL Update's RowsAffected guard (only reachable if a session is deleted between Get and UPDATE, which the single-connection SQLite pool cannot interleave -- same blind spot as main's f5870ec) and the empty-token guard (every backend already refuses "" structurally: NULL in SQL, an absent field in Mongo, never indexed in memory). End-to-end: the built server on INDRI_DB_BACKEND=sqlite registered and logged in a user with no MongoDB, kept user and session across a restart, refused a wrong password, and logged the timer warning; on memory with the tictactoe config, create seated the caller in p0 as host with p1 free, and the old userId kick payload was refused.

### 2026-10-03T05:31:02.000Z - REVIEW. An independent review found one critical defect, now fixed at the root: register decodes the client's payload into models.CreateUser, whose ID was json:"id", and stores honoured a supplied id -- so on the memory backend a registration naming a victim's (game-visible) user id replaced their account and inherited their live session. CreateUser and CreateSession no longer carry an id; stores always mint (test: register cannot choose the user id, red then green). Also fixed: SQL Update was an unlocked read-modify-write and lost concurrent updates to different fields on both SQL dialects (tests red on sqlite and postgres) -- now one transaction with the row locked via sqldialect.LockForUpdate; the 080 game store's mutateStored lost concurrent UpdateField writes on PostgreSQL the same way (new test red on postgres) and takes the same lock; sessions lift game_id into an indexed column so a broadcast no longer decodes the whole table (test strengthened to cover moves in and out of a game and expired sessions; dropping the column write fails it); memory or sqlite with INDRI_LOCK_BACKEND=redis is refused at boot; CLAUDE.md no longer claims the game store maps ErrDuplicate. The SQL RowsAffected guard that survived mutation is gone -- the locked transaction makes it unnecessary. Filed: 092 (SQL game stores drop empty data maps, so UpdateField under them fails where MongoDB succeeds -- found writing the UpdateField test, pre-existing from 080). Not acted on: Mongo contract tests leave users in indri_test (pre-existing pattern for games too).

### 2026-10-03T05:31:02.000Z - PROOF. completeness: user and session have Mongo, memory and SQL stores held to one contract each; services and the injector use Storer; INDRI_DB_BACKEND selects all three. feature-availability: a SQLite server registers, logs in and persists across a restart with no MongoDB. robustness/resilience: misconfiguration fails at boot naming the variable; the SQL pool closes on shutdown. security: ids are never client-chosen; tokens and passwords live outside the JSON documents and survive every read; tokens die at sessionMaxAge on every backend including Mongo reads; GetByToken("") refused. defense-in-depth: unknown lookup keys are errors; token lookups only through GetByToken. input-validation: missing user id, missing token, unknown key, unknown backend. thread-safety: racing registrations and logins hold one-per-email and one-per-user; concurrent field updates no longer lost on any backend, under -race. configurability: INDRI_DB_BACKEND, INDRI_SQLITE_PATH, INDRI_POSTGRES_URI documented in .env.example. Final: go build/vet/gofmt clean; go test -race -count=1 ./... green with MongoDB and PostgreSQL, 0 skips.

