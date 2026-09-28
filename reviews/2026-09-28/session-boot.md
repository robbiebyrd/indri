# Review slice: sessions, users, config, clients, injector, boot, Docker, CI

Reviewed at worktree `wip/codebase-review` (HEAD 6330d92, code identical to main 3eab208). The review was read-only.

**Verification run** (GOCACHE was pointed at the scratchpad because the sandbox blocks `~/Library/Caches`):
- `go vet` on all slice packages: clean.
- `go test -race` on `repo/session`, `repo/user`, `repo/env`, `clients/postgres`, `clients/sqlite`, `injector`, `cli` and `services/boot`: all pass.
  - The memory and SQLite tests ran.
  - The **Mongo tests ran**, because a local MongoDB was reachable. `TestStore_ExpiredSessionIsGone/mongo` is skipped on purpose.
  - **Every Postgres test was SKIPPED** because `INDRI_TEST_POSTGRES_URI` is unset. That includes `TestOpen_CapsOpenConnections`.
- `repo/script`, `clients/mongodb`, `clients/redis` and `cmd/server` have **no test files**.

---

## Prior findings re-check

| Prior | Topic | Status | Evidence |
|---|---|---|---|
| SEC-1 (Sep 9) | Logout must invalidate the token | **FIXED** | `internal/handlers/actions/logout/handler.go:40` deletes the session. Side effect: sessions are shared per user, so logout on one device also kills every other device's token (see SB-3). |
| #c546 (Sep 9) | session.New duplicate-key race | **CHANGED**: fixed on Mongo/Postgres, still open on SQLite | Mongo `session/mongo.go:186-188` and Postgres `session/postgres.go:102-105` re-read the winner. SQLite `session/sqlite.go:81-110` does SELECT then INSERT with no retry. See SB-7. |
| #7d48 (Sep 9) | Dockerfile listen address | **FIXED** | `Dockerfile:18` `ENV INDRI_LISTEN_ADDRESS=0.0.0.0`. The image still cannot start, though (SB-4). |
| 6760eda | Mongo URI logged | **FIXED** | `clients/mongodb/client.go:31` logs only the DB name. `injector/repos.go:78` keeps the Postgres URI out of errors. |
| 086c336 | `log.Fatal` in Boot | **CHANGED** | Boot itself returns errors, but `repo/script/script.go:20,27` still calls `log.Fatalf` from a constructor that is reached via `injector/repos.go:99` (SB-16). |
| A #3 | Session expiry on SQLite/memory | **FIXED** (base). **Follow-up STILL-OPEN** | SQLite: `session/sqlite.go:117,134,148,160`. Memory: `session/memory.go:35-37,95,108`. Boss's follow-up (24 h *idle* expiry on every store, plus an on-read check on Mongo) is **not implemented**: `session/mongo.go:25` is still a 7-day absolute limit, and Mongo `Get/GetByToken/Find/FindFirst/Exists/New` (`mongo.go:85-137`) never filter on `createdAt`. See SB-5 and SB-9. |
| A #4 | Mongo ObjectID→string, no migration | **STILL-OPEN** (accepted/deferred) | Lookups filter `_id` by a raw string (`session/mongo.go:109,125,131,157`, `user/mongo.go:104,109,128`). No migration or release note exists. The docs still describe ObjectIDs: `CLAUDE.md:207` (`session.ID.Hex()` (Mongo ObjectID)) and `docs/PROTOCOL.md:124` (SB-22). |
| A #5 | SQL session Update rewrites the whole blob | **CHANGED** | RowsAffected→ErrNotFound and the `user_id` column are fixed (`session/sqlite.go:211-232`, `postgres.go:241-262`). The Get→patch→write-whole-blob with no fence is **still there**, but no current caller can reach it: the only callers (`handlers/actions/create/handler.go:79`, `join/handler.go:77`) always set all four fields, which gives the same last-writer-wins as Mongo's `$set`. It becomes a live race once session-resume adds partial updates or field clearing (SB-12). |
| A #9 | SQLite Find full scan | **STILL-OPEN** | `session/sqlite.go:158-182` ignores `key` in SQL and decodes every row. `user/sqlite.go:129-157` does the same, and it is on every login path (`authentication.go:42`). Pool pinned at 1 (`clients/sqlite/client.go:53`). See SB-6. |
| A (extra) | SQL pool not closed when a later constructor fails | **STILL-OPEN** | `injector/repos.go:65-76` and `83-94`, plus the script store at `99-101`, all `return nil, err` without `sqlDB.Close()` (SB-18). |
| A (extra) | `repo.ErrConflict` unused | **STILL-OPEN** | `internal/repo/errors.go:11`. No reference anywhere (rg). |
| A (extra) | `isPgDuplicateKey` copied, matched on text | **STILL-OPEN** | `session/postgres.go:56-62`, `user/postgres.go:51-57`, and a third copy in the game store (SB-21). |
| 2026-09-28 | SQLite has no `busy_timeout` | **STILL-OPEN** | `clients/sqlite/client.go:55-63` sets only `journal_mode=WAL` and `foreign_keys` (SB-14). |
| 2026-09-28 | Postgres DDL duplicates UNIQUE indexes | **STILL-OPEN (confirmed)** | `session/postgres.go:20-21` has `token`/`user_id` UNIQUE, plus redundant `idx_sessions_token` and `idx_sessions_user_id` (`:26-27`). `user/postgres.go:23` has `email UNIQUE`, plus `idx_users_email` (`:27`). Also `game/postgres.go:22,27` (`code UNIQUE` plus `idx_games_code`, outside this slice) (SB-19). |
| 2026-09-28 | Postgres rejects NUL (`\u0000`) | **STILL-OPEN, PLAUSIBLE** (not executed; Postgres is unavailable here) | No store strips or rejects NUL. Postgres documents that `jsonb` rejects `\u0000` and `text` rejects 0x00 (SB-20). |
| Review B leftover | SQL session `Find("gameId")` full scan | **STILL-OPEN** | Postgres `session/postgres.go:182-183,195-214` (`findInGo`), SQLite `sqlite.go:158`. It runs on **every delta broadcast** via `broadcast.go:72` (`BroadcastLocal` → `gameSessions` → `Find("gameId")`) (SB-6). |
| 2026-09-28 | CI Postgres job never confirmed green | **UNKNOWN** | Could not run it here. CI has **no Mongo or Redis service** (`.github/workflows/ci.yml:12-28`), so the Mongo stores and the Redis lock/bus tests always skip in CI (SB-17). |

---

## Findings (severity-sorted)

### [P2] Plaintext passwords and live bearer tokens are written to the server log on any failed auth message
- ID: SB-1
- Category: security
- Location: `internal/handlers/router/router.go:16` and `:24`. Triggered via `internal/services/authentication/authentication.go:42-51`, `internal/handlers/actions/register/handler.go:50-53` and `internal/handlers/actions/reconnect/handler.go:40-46`.
- Confidence: CONFIRMED
- Prior: new
- What: `HandleMessage` logs the whole decoded payload whenever a handler returns an error (`log.Printf("error handling message %v: %v\n", decodedMsg, err)`), and logs the raw bytes when decoding fails. The payloads of `login` and `register` contain `password`, and the payload of `reconnect` contains the bearer token under `sessionId`.
- Failure scenario:
  - A user mistypes their password. `Authenticate` returns "password does not match", and the log gets `error handling message map[email:alice@x password:<what they typed>]`. That is usually the real password or a near miss.
  - `register` with an existing email logs the new password in cleartext.
  - `reconnect` on an already-authenticated connection logs a **valid** 256-bit token. Anyone with log access can replay that token for up to 7 days.
- Suggested fix: never log payloads. At most log the action name, or redact `password`/`sessionId`/`token` keys before logging. Also drop `string(msg)` from the decode-error log.

### [P2] Graceful shutdown never records players as disconnected on persistent backends (ghost presence after every deploy)
- ID: SB-2
- Category: bug
- Location:
  - The root ctx is captured once at boot (`cmd/server/main.go:23`, `internal/injector/repos.go:35-94`) and stored by every store: `session/mongo.go:73`, `session/sqlite.go:30`, `session/postgres.go:51` (and the game stores).
  - The shutdown path is `internal/entrypoints/http.go:29-36` → `Transport.Close` → `Disconnect` → `internal/entrypoints/websocket.go:45` (`ss.Get`) and `:57` (`DisconnectPlayer`).
- Confidence: CONFIRMED (by code path; the Mongo driver and `database/sql` both return `ctx.Err()` on an already-cancelled context)
- Prior: new
- What: SIGINT/SIGTERM cancels the root ctx, and only then is `Transport.Close()` called. Closing the transport fires `HandleDisconnect` for each connection. The session lookup and the game `Mutate` run on the stores' saved ctx, which is already cancelled, so they fail immediately with `context canceled`. The only trace is a log line ("error getting session").
- Failure scenario: 20 players are in games on Mongo, SQLite or Postgres. The operator restarts the server. Every player's slot keeps `connected: true` in the database. After the restart, clients see players as present who are not, and any "all players connected" or host-presence logic is wrong until each player reconnects and disconnects again. The memory backend is not affected, but it loses the data anyway.
- Suggested fix: give stores a per-call ctx (`context.WithoutCancel(root)` plus a timeout for shutdown-time cleanup), or run the disconnect cleanup with a fresh bounded ctx before cancelling the store ctx. Longer term, pass a request ctx through the Storer methods.

### [P2] Login never rotates the token or extends the session: it returns the existing session unchanged and discards the new token
- ID: SB-3
- Category: security
- Location: `internal/services/authentication/authentication.go:53-65`. Returns-existing paths: `session/memory.go:79-81`, `session/sqlite.go:85-87`, `session/postgres.go:81-84`, `session/mongo.go:85-94`.
- Confidence: CONFIRMED
- Prior: new (related to A #3 follow-up)
- What: `Authenticate` generates a fresh 256-bit token and calls `sessionRepo.New`. Every store's `New` returns the user's *existing* session when there is one, and that session's old token and old `createdAt` go with it. The freshly generated token is silently thrown away. As a result:
  1. A password login never rotates the bearer token. If a token leaks, logging in again does not revoke it; only `logout` (which kills every device) or the 7-day cap does.
  2. Logging in does not reset the expiry clock.
  3. Because every device shares the one token, `logout` on one device invalidates all of them.
- Failure scenario: A user logs in on Monday 10:00 and plays daily; each day they log in again with their password. On the following Monday at 10:00 the session hits `sessionMaxAge`. They logged in 5 minutes earlier, but mid-game every action suddenly fails with not-found and `reconnect` is refused. The server handed them a token with 5 minutes left.
- Suggested fix: on a successful password login, rotate: update the existing session's token and reset `createdAt`/last-seen atomically, or delete and recreate it. This fits the planned 24 h idle-expiry work. Decide explicitly whether sessions are per-user or per-device.

### [P2] The Docker image exits immediately: the CMD passes no `-script`, and the binary requires one
- ID: SB-4
- Category: bug
- Location: `Dockerfile:24` (`CMD ["/app/main"]`), `cmd/server/main.go:16` (`cli.Parse(..., "")`), `internal/cli/cli.go:30-32`
- Confidence: CONFIRMED
- Prior: new (#7d48 fixed the listen address only)
- What: `cmd/server` passes an empty default script, so `cli.Parse` returns "a script file is required: use -script <path>" and `main` calls `log.Fatal`.
- Failure scenario: `docker build . && docker run -p 5002:5002 <img>` exits with status 1 on startup, every time.
- Suggested fix: `CMD ["/app/main", "-script", "/app/config.json"]` (or an ENTRYPOINT plus a default arg). While there (SB-15): use a multi-stage build and a non-root user.

### [P2] Session and user lookups on the SQL backends scan the whole table on hot paths (every delta broadcast, every login)
- ID: SB-6
- Category: performance
- Location:
  - `session/sqlite.go:158-182` (ignores the key entirely)
  - `session/postgres.go:175-214` (`gameId`/`teamId` fall back to `findInGo`)
  - `user/sqlite.go:129-157` (scans all users even for `email`, which is an indexed UNIQUE column)
  - Callers: `services/broadcast/broadcast.go:72,152,163,182,196`, `services/authentication/authentication.go:42`
- Confidence: CONFIRMED
- Prior: A #9 plus the Review B leftover (STILL-OPEN)
- What: each call does `SELECT data FROM sessions` (or `users`), JSON-decodes every row, and filters in Go. `BroadcastLocal` runs `Find("gameId")` once for every change event.
- Failure scenario: a SQLite deployment with 5,000 live sessions. Each tic-tac-toe move publishes a delta, and the monitor decodes 5,000 JSON blobs to find the 2 recipients, all on the single pinned SQLite connection. That starves every other query. Each login on SQLite likewise decodes every user row.
- Suggested fix:
  - For session `token`/`userId` and user `email`, query the existing indexed columns.
  - For `gameId`/`teamId`, add `game_id`/`team_id` columns (kept in `Update`) with an index. On Postgres, a `data->>'gameId'` expression index also works.

### [P3] Mongo does not enforce session expiry on read, and the agreed 24 h idle expiry is not implemented
- ID: SB-5
- Category: security
- Location: `session/mongo.go:25`, `:85-137`. `store_contract_test.go:186` skips the expiry contract for Mongo.
- Confidence: CONFIRMED
- Prior: A #3 follow-up (STILL-OPEN)
- What: memory, SQLite and Postgres filter `created_at > cutoff` on every read. Mongo relies only on the TTL monitor, which runs about every 60 s and can lag further under load. Mongo `New` also returns an expired-but-unswept session. No store has idle (last-seen) expiry.
- Failure scenario: a leaked token still works in `reconnect` via `GetByToken` for up to the sweep delay after its 7-day expiry. Separately, `login` in that window returns the dying session (SB-3 amplifies this), and the user is logged out within a minute.
- Suggested fix: add `createdAt > cutoff` (later, `lastSeen > cutoff`) to every Mongo read filter, and have `New` ignore or delete stale rows. Implement the idle expiry across all four stores in shared `fields.go` logic, and remove the Mongo skip from the contract test.

### [P3] SQLite session `New` fails under concurrent logins for the same user
- ID: SB-7
- Category: concurrency
- Location: `session/sqlite.go:81-110`
- Confidence: CONFIRMED (by code; not executed, because the review was read-only)
- Prior: #c546 (fixed for Mongo/Postgres only)
- What: SELECT-by-user_id, then INSERT, with no transaction and no handling of a UNIQUE violation. The single pooled connection serializes individual statements, not the pair. Mongo (`mongo.go:186`) and Postgres (`postgres.go:102`) re-read the winner when they hit a duplicate; SQLite returns `insert session: UNIQUE constraint failed`.
- Failure scenario: the same user logs in from two tabs at once, or double-submits the login. Both goroutines see no row, one INSERT wins, and the other login fails. The failure is silent to the client, because handler errors are only logged.
- Suggested fix: on `sqliteClient.IsUniqueViolation(err)`, re-select by `user_id` and return that row (mirror Postgres), or use `INSERT ... ON CONFLICT(user_id) DO NOTHING` and then SELECT.

### [P3] A malformed setting half-applies the configuration: envconfig stops at the first bad value and later fields stay zero
- ID: SB-8
- Category: bug
- Location: `internal/repo/env/env.go:80-82`. See also `envconfig@v1.4.0/envconfig.go` `Process`, which returns on the first `ParseError`.
- Confidence: CONFIRMED (by reading the envconfig source)
- Prior: new
- What: the `envconfig.Process` error is only logged, and loading continues. `Process` walks the fields in declaration order and returns at the first parse failure, so every field *after* it keeps Go's zero value, not its `default:` tag. The same happens when a JSON config value is not an integer (e.g. `"listenPort": 5002.5`), because JSON values are pushed through env.
- Failure scenario:
  - `INDRI_REDIS_PORT=6379/tcp` or a typo'd `INDRI_LISTEN_PORT`: `LockBackend`, `DBBackend`, `Transports` and the rest become `""`/`0`. Boot then fails with the misleading "unknown database backend \"\"".
  - A bad late field (e.g. `INDRI_WEBRTC_MAX_PEERS=abc`) leaves the later WebRTC/GraphQL timeouts at 0. With `ws` only, the server boots silently misconfigured.
- Suggested fix: return the `envconfig` error from `load` and fail boot on it. Add range validation (ports, positive timeouts) in one place.

### [P3] `INDRI_WS_PING_PERIOD=0` (or `-ws-ping-period 0`) crashes the process on the first WebSocket connection
- ID: SB-10
- Category: bug
- Location: `internal/repo/env/env.go:31` (no validation), `internal/transport/ws/ws.go:47`, `melody@v1.3.0/session.go:75` (`time.NewTicker(PingPeriod)`)
- Confidence: CONFIRMED (by code; `time.NewTicker` panics on non-positive intervals, in a goroutine where no recover runs)
- Prior: new
- What: the GraphQL and WebRTC settings are validated as `<= 0` (`injector/transport.go:67,91`), but the ws settings are not. An operator who sets 0 to "disable pings", or a zero from SB-8, gets a server that boots and then panics.
- Failure scenario: `INDRI_WS_PING_PERIOD=0`. The first client connects, melody's `writePump` calls `time.NewTicker(0)`, and the panic kills the whole server.
- Suggested fix: validate `WSPingPeriodSeconds > 0`, `WSPongTimeoutSeconds > WSPingPeriodSeconds`, `WSWriteTimeout > 0` and `WSMaxMessageSizeBytes > 0` at load or in `buildTransport("ws")`.

### [P3] User enumeration: register answers only on success, and login skips bcrypt for unknown emails
- ID: SB-11
- Category: security
- Location: `internal/handlers/actions/register/handler.go:50-55`, `internal/services/authentication/authentication.go:42-51`
- Confidence: CONFIRMED
- Prior: new
- What:
  - `register` writes `{registered:true}` on success and nothing on a duplicate email (the error is only logged), so the response itself reveals whether an email exists.
  - `Authenticate` returns immediately when the email is unknown, but runs a ~cost-10 bcrypt when it exists. Messages on a connection are processed serially, so an attacker can time `login` by sending a follow-up message.
  - There is no rate limiting on either action; each `login` for a real email costs one bcrypt (CPU amplification).
- Failure scenario: an attacker scripts `register` with a candidate email list and gets a response only for unregistered addresses.
- Suggested fix:
  - Return a uniform response for register.
  - In `Authenticate`, compare against a dummy bcrypt hash when the user is missing.
  - Add per-connection/IP throttling for `login`/`register`.

### [P3] The SQL session `Update` is a read-modify-write of the whole blob with no fence, and "empty means unchanged" means a field can never be cleared
- ID: SB-12
- Category: concurrency
- Location: `session/sqlite.go:195-234`, `session/postgres.go:227-264`, `session/fields.go:30-44`
- Confidence: CONFIRMED (the code shape). Not reachable today: see the prior table, A #5.
- Prior: A #5 (remaining part)
- What: `Get` → `applyUpdate` → `UPDATE ... SET data=<whole blob>` has no version or `WHERE data = old`. `applyUpdate` also skips empty strings, so no store can clear GameID/TeamID/SlotID. The session-resume plan's "bug 2" needs exactly that.
- Failure scenario: once any caller issues a partial update (e.g. session-resume clearing SlotID on leave while `join` sets GameID), SQLite/Postgres lose one of the two writes, while Mongo's per-field `$set` keeps both.
- Suggested fix: per-field SQL updates (`jsonb_set` on Postgres, `json_set` on SQLite), or a version column. Add an explicit clear operation to `UpdateSession`.

### [P3] The Postgres pool keeps only 2 idle connections, so connections churn under bursty load
- ID: SB-13
- Category: performance
- Location: `internal/clients/postgres/client.go:31`
- Confidence: CONFIRMED (by code; `database/sql`'s default `MaxIdleConns` is 2)
- Prior: new
- What: only `SetMaxOpenConns(25)` is set, with no `SetMaxIdleConns`, `SetConnMaxLifetime` or `SetConnMaxIdleTime`.
- Failure scenario: a broadcast burst opens around 25 connections. As soon as they are released, all but 2 are closed, and the next burst pays TCP+TLS+auth setup again, adding latency to every mutation during play.
- Suggested fix: `SetMaxIdleConns(MaxOpenConns)` plus a sensible `SetConnMaxIdleTime`/`SetConnMaxLifetime`.

### [P3] SQLite has no `busy_timeout`, and its single-connection pool makes every query wait behind every other
- ID: SB-14
- Category: bug
- Location: `internal/clients/sqlite/client.go:53-63`
- Confidence: CONFIRMED
- Prior: 2026-09-28 leftover (STILL-OPEN)
- What: there is no `PRAGMA busy_timeout`. Any second process on the same file (a second instance, or an admin `sqlite3` shell holding a write lock) gets `SQLITE_BUSY` immediately. `SetMaxOpenConns(1)` also defeats the WAL "concurrent readers" rationale stated at `:55`.
- Failure scenario: an operator runs `sqlite3 indri.db` and starts a write transaction. Every game write on the server fails at once with "database is locked", instead of waiting.
- Suggested fix: add `_pragma=busy_timeout(5000)` to the DSN (applied per connection by modernc). Consider a larger read pool for file DSNs.

### [P3] Docker and Compose hardening: root user, full toolchain in the image, over-broad build context, and unauthenticated DBs on all interfaces
- ID: SB-15
- Category: security
- Location: `Dockerfile:2-24`, `.dockerignore:1-14`, `docker-compose.yml:3-27`, `.env.example:9,15`
- Confidence: CONFIRMED (a PLAUSIBLE part is marked below)
- Prior: new
- What:
  - **Dockerfile:**
    - The image is `golang:1.25-bookworm`: a single-stage build that ships the source and the Go toolchain, and runs as **root**.
    - `COPY . .` uses a `.dockerignore` whose `.env` pattern is root-anchored only. `client/.env` exists in the main checkout and is copied into the image. `.worktrees/`, `.pnpm-store/` (present in the main checkout) and `*.db` / `indri.db*` are not ignored either.
    - With `INDRI_DB_BACKEND=sqlite` and the default `./indri.db`, a local DB holding password hashes and live tokens gets baked into the image. It is not in `.gitignore` either, so it can be committed by accident.
  - **Compose:**
    - Mongo is published on `0.0.0.0:27017` with no root user configured: `.env.example` sets no `MONGO_INITDB_ROOT_*`.
    - Redis is `bitnami/redis:latest` (unpinned) with `INDRI_REDIS_EMPTY_PASSWORD="yes"` in `.env.example`, published on `0.0.0.0:6379`.
- Failure scenario: a developer runs `docker compose up -d` on a laptop on café Wi-Fi. Anyone on the LAN can read and write the `user` and `session` collections, including live bearer tokens.
- Suggested fix:
  - Multi-stage build → distroless/alpine, with `USER nonroot`.
  - Add `**/.env`, `**/.env.*`, `.worktrees`, `.pnpm-store`, `*.db*` and `client/` to `.dockerignore`, and `*.db*` to `.gitignore`.
  - Bind compose ports to `127.0.0.1`, set Mongo root credentials, pin the Redis image, and default to requiring a Redis password.
- PLAUSIBLE: Bitnami stopped publishing free `bitnami/*:latest` updates in 2025, so this image may be frozen or removed. Not verified here.

### [P3] The script loader calls `log.Fatalf` inside a constructor that returns an error
- ID: SB-16
- Category: maintainability
- Location: `internal/repo/script/script.go:18-30`
- Confidence: CONFIRMED
- Prior: 086c336 ("log.Fatal in Boot"), partly fixed
- What: `NewStore` exits the process on a read or parse error, and its `error` return is dead (always nil). A script JSON typo kills the process from inside `injector.GetRepos`. This bypasses `Boot`'s error path, so no deferred cleanup runs, the `boot.Boot` error contract is broken for embedders and tests, and the error never reaches the documented caller-facing wrapper.
- Failure scenario: a test or an embedding binary calls `boot.Boot` with a malformed script and the whole test binary exits instead of receiving an error.
- Suggested fix: return `fmt.Errorf("reading script %q: %w", ...)` and `fmt.Errorf("parsing script ...: %w", ...)`.

### [P3] Test and CI gaps for the persistence and boot layers
- ID: SB-17
- Category: tests
- Location: `.github/workflows/ci.yml:12-28`, `internal/repo/user/*_test.go`, `internal/repo/session/store_contract_test.go:186`
- Confidence: CONFIRMED
- Prior: new
- What:
  - CI has no Mongo or Redis service, so `session/mongo_test.go`, `game/mongo_concurrency_test.go` and `events/redis_test.go` always skip there. The default backend (Mongo) and the multi-instance mode (Redis) are never tested in CI.
  - The user store has **no contract test** (sessions and games have one), and the **Mongo user store has no tests at all**. That is how the divergences in SB-21 went unnoticed.
  - No tests cover `clients/mongodb`, `clients/redis`, `repo/script` or `cmd/server`.
  - No test covers env parse failures (SB-8), shutdown with a persistent store (SB-2), concurrent SQLite `New` (SB-7), or `Authenticate`'s behaviour when a session already exists (SB-3).
  - Actions are pinned by tag, not SHA, and there is no `govulncheck` step.
- Failure scenario: a regression in `MongoStore` (the production default) merges with CI green.
- Suggested fix: add `mongo:8` and `redis:7` services to CI with `INDRI_TEST_MONGO_URI`/`INDRI_TEST_REDIS_URL`, and a `user` `eachStore` contract suite. Add `govulncheck ./...`.

### [P4] The SQL pool and the Mongo client leak when a later boot step fails
- ID: SB-18
- Category: maintainability
- Location: `internal/injector/repos.go:61-101`, `internal/clients/mongodb/client.go:33-41`, `internal/injector/clients.go:26-58`, `internal/services/boot/boot.go:24-37`
- Confidence: CONFIRMED
- Prior: A extra "SQL pool not closed" (STILL-OPEN)
- What:
  - `repos.go` returns without `sqlDB.Close()` when a store constructor or `scriptRepo.NewStore` fails.
  - `mongodb.New` does not `Disconnect` after a failed `Ping`.
  - `Boot` does not release clients when repos or services fail.
  - The shared Redis client (`clients.go:49-58`) is never stored on the injector, so `closeResources` (`serve.go:35-53`) never closes it, even on a clean shutdown.
- Failure scenario: in `main`, `log.Fatalf` hides these leaks. A test harness or embedder that retries `Boot` accumulates open pools and Redis connections.
- Suggested fix: close on the error paths (a `defer` guarded by a success flag). Keep `*redis.Client` on `ClientsInjector` and close it in `closeResources`.

### [P4] The Postgres DDL creates redundant indexes on columns that are already UNIQUE
- ID: SB-19
- Category: performance
- Location: `session/postgres.go:26-27`, `user/postgres.go:27` (and `game/postgres.go:27`)
- Confidence: CONFIRMED
- Prior: 2026-09-28 leftover (now verified)
- What: a `UNIQUE` column already has a unique B-tree index. `idx_sessions_token`, `idx_sessions_user_id` and `idx_users_email` duplicate them.
- Failure scenario: every session insert and update maintains 2 extra indexes (write amplification and storage) for no read benefit.
- Suggested fix: drop the three `CREATE INDEX` lines, and add `DROP INDEX IF EXISTS` for existing databases.

### [P4] Postgres rejects NUL characters that the other backends accept
- ID: SB-20
- Category: bug
- Location: `user/postgres.go:74-77`, `session/postgres.go:97-100` (the game store likewise)
- Confidence: PLAUSIBLE (Postgres documents this behaviour; not executed here)
- Prior: 2026-09-28 leftover
- What: `jsonb` rejects the `\u0000` escape, and `text` rejects 0x00 bytes. Nothing strips or validates NUL before a write.
- Failure scenario: `register` with `"name":"a\u0000b"` succeeds on Mongo, memory and SQLite but fails on Postgres with an opaque driver error. The same happens for game payloads.
- Suggested fix: reject NUL at the input boundary (the handler or service), so every backend behaves the same.

### [P4] Cross-store divergence and duplication in the user and session stores
- ID: SB-21
- Category: maintainability
- Location:
  - `user/mongo.go:118-139` (Update)
  - `user/mongo.go:93-100` vs `user/memory.go:133-142` (Find keys)
  - `session/mongo.go:98-119` vs `session/memory.go:181-194`
  - `user/sqlite.go:203-213`
  - `session/mongo.go:85`, `:196-202`, `:30`
  - `session/sqlite.go:36-60`
  - `session/postgres.go:112-162,271-291`
  - `session/{mongo,memory,sqlite,postgres}.go` (the four `FindFirst`s)
- Confidence: CONFIRMED
- Prior: partly A extra (`isPgDuplicateKey` copies)
- What:
  - **Mongo user `Update`:**
    - Uses `context.TODO()` instead of the store ctx.
    - Returns a plain "does not exists" error instead of wrapping `ErrNotFound`.
    - Does not map a duplicate email to `ErrDuplicate`, whereas memory, SQLite and Postgres do.
  - **Missing records:** Mongo `Get`/`FindFirst`/`GetByToken` return `mongo.ErrNoDocuments`, not `repo.ErrNotFound`, so `errors.Is(err, ErrNotFound)` works on three backends but not the default one.
  - **Find keys:** Mongo `Find` accepts any field (e.g. `slotId`), while the other stores return nothing for keys outside their `match*Field` switch.
  - **SQLite user `Update`:** ignores `RowsAffected`.
  - **Mongo session `New`:** discards the `FindOne` error (`matchingSession, _ :=`).
  - **Dead code:** `isSessionInGameAndTeam`/`isSessionInGame` and the unused `client` field.
  - **Duplicated code:**
    - The `sessionBlob` type is declared twice in `sqlite.go`.
    - The Postgres `Get`/`GetByToken` bodies duplicate `scanSessions`.
    - `FindFirst` is written out 4 times per repo.
  - **Misplaced constant:** `sessionMaxAge` lives in `mongo.go` but is used by all stores.
  - **Copy-paste errors:** `authentication.go:26` ("userRepo is required" for sessionRepo), `services.go:16,20`.
  - **Unused settings and indexes:** `MongoAuthDatabase` (`env.go:29`) is never read; the Mongo `score` index (`user/mongo.go:42`) indexes a field no code writes.
- Failure scenario: a caller written against memory or SQL (e.g. session-resume checking `errors.Is(err, repo.ErrNotFound)` after `Get`) misbehaves only in production on Mongo.
- Suggested fix:
  - Wrap Mongo not-found and duplicate errors in the sentinels.
  - Make `Find` accept the same whitelisted keys in every store.
  - Move the shared helpers (`FindFirst`, blob marshal, max age) into `fields.go`.
  - Add the user contract test (SB-17).

### [P4] Documentation and sample-config drift
- ID: SB-22
- Category: docs
- Location:
  - `CLAUDE.md:49-50`, `README.md:51-52`
  - `docs/ARCHITECTURE.md:193-194`
  - `CLAUDE.md:207`, `docs/PROTOCOL.md:124`
  - CLAUDE.md "Clients are process-global singletons; GetClients returns a cached *ClientsInjector" vs `injector/clients.go:18-23`
  - `env.go:27`, `docs/ARCHITECTURE.md:289`
  - `.env.example:15`, `example/server.json:9`
  - `.env.example:17`
- Confidence: CONFIRMED (the `mongodb+srv` part is PLAUSIBLE)
- Prior: related to A #4 (release note open)
- What:
  - The docs say compose starts an `rs0` replica set, but `docker-compose.yml:8-10` is standalone.
  - ARCHITECTURE says memory and SQLite do not expire sessions; they do now.
  - The docs describe session IDs as Mongo ObjectIDs (`.Hex()`); they are UUID strings (`repo/ids/ids.go`).
  - `globalClientsInjector` is never assigned, so the documented cache does not exist and every `GetClients` call builds new clients.
  - The default `INDRI_MONGO_URI` of `localhost` has no scheme, so the Go driver rejects it.
  - The sample URI `mongodb+srv://test:test@localhost/` needs an SRV record for `localhost`, which does not exist (PLAUSIBLE; not run).
  - `INDRI_MONGO_AUTH_DATABASE` is documented but has no effect.
- Failure scenario: a new developer copies `.env.example` and runs `go run ./cmd/server -script config.json`. The Mongo connect fails, and the docs point them at a replica set that isn't there.
- Suggested fix:
  - Fix the prose.
  - Default the URI to `mongodb://localhost:27017`.
  - Either assign `globalClientsInjector` or delete the cache and the claim.
  - Remove `MongoAuthDatabase`, or wire it to `options.Credential.AuthSource`.
  - Add the ObjectID→UUID release note.

### [P4] The Mongo TTL index cannot be changed in place: changing `sessionMaxAge` will fail boot on existing databases
- ID: SB-9
- Category: bug
- Location: `session/mongo.go:46-51,67-70`
- Confidence: PLAUSIBLE (standard MongoDB behaviour: `createIndexes` on the same key pattern with different options errors with `IndexOptionsConflict`; not executed)
- Prior: related to the A #3 follow-up
- What: the planned move from 7 d to 24 h changes `expireAfterSeconds` on the existing `{createdAt:1}` index. `CreateMany` will return an error, and `NewMongoStore` fails boot.
- Failure scenario: session-resume ships 24 h expiry, and every existing Mongo deployment refuses to start with "initializing repos: … IndexOptionsConflict".
- Suggested fix: use `collMod` to update `expireAfterSeconds`, or drop and recreate the index when the options differ. Better still, move to a `lastSeen` field with its own index plus on-read checks (SB-5).
