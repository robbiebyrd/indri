# Indri architecture

A component map with file references. For the wire format see [PROTOCOL.md](PROTOCOL.md); for
day-to-day commands and conventions see [../CLAUDE.md](../CLAUDE.md).

## Request lifecycle

```
browser / native client
        │  /ws, /sse/*, /graphql, /webrtc/offer   (whichever INDRI_TRANSPORTS enables)
        ▼
internal/entrypoints/http.go             net/http mux, timeouts, graceful shutdown
        ▼
internal/transport/<kind>/               one Transport per wire protocol; Composite runs several at once
        ▼
internal/services/boot/handlers.go       HandleConnect / HandleMessage / HandleDisconnect / HandleError
        ▼
internal/handlers/router/router.go       decode JSON, extract + strip "action"
        ▼
internal/handlers/router/act.go          run "received" → <action> → "processed"; recover() per handler
        ▼
internal/handlers/actions/<action>/      one package per action
        ▼
internal/services/*                      business logic
        ▼
internal/repo/*                          MongoDB stores
        │
        ├── writes ──► MongoDB
        │
        ├── diff  ──► internal/services/events/      Diff(before, after) → ChangeEvent
        │                   │  Publish
        │                   ▼
        │             in-process channel, or Redis Pub/Sub (multi-instance)
        │                   ▼
        │       internal/services/boot/monitor.go    Subscribe, fan out per game
        │                   ▼
        └────── internal/services/broadcast/         BroadcastLocal → this instance's clients
```

Note the delta path is driven by the **application**, not by MongoDB. Because the server is the sole
writer, it derives each delta at the write site; there is no change stream and no replica-set
requirement.

## Layers

### `cmd/server`

Thin `main`: parse `-script`, install a SIGINT/SIGTERM-cancelled root context, `boot.Boot`, `boot.Serve`.
`example/tictactoe/main.go` is the same, plus one `router.RegisterHandler` call — that is the entire
delta between "the framework" and "a game".

### `internal/injector`

Three-stage construction, each stage depending only on the previous one.

| File | Builds |
|---|---|
| `clients.go` | Mongo client (`mongodb` backend only), client transport (built from `INDRI_TRANSPORTS` by `transport.go`), lock manager, change-event publisher. Cached process-globally; all four are injectable for tests. |
| `repos.go` | `game`, `user`, `session` stores for the backend chosen by `INDRI_DB_BACKEND` (see [Database backends](#database-backends)) plus the script store. |
| `services.go` | game, broadcast, auth, user, session services. |

`injector.Injector` embeds all three structs plus `GlobalContext` and the parsed `Script`, and is the
single value threaded into every handler.

`INDRI_LOCK_BACKEND` is really a multi-instance switch. Set to `redis`, `GetClients` opens one Redis
connection and shares it between `lock.Redis`, the change bus (`events.NewRedis`, `indri:changes`), and
the delivery relay (`events.NewRedisDeliveries`, `indri:deliveries`); otherwise locks and changes are
in-process and there is no relay.

Sends to other players (`BroadcastService.Broadcast`, `CloseSessions`) resolve their sessions on the
sending instance, then publish an `events.Delivery` on the relay; every instance's
`BroadcastService.RelayDeliveries` (run by `boot.Serve`) applies it to the connections it holds. Change
events already reach every instance, so the monitor delivers them with `BroadcastLocal` instead.

### `internal/entrypoints`

- `http.go` — mounts the transport's routes and runs the HTTP server, with read/write/idle timeouts.
  On context cancellation it closes the transport *first* (so hijacked sockets and SSE streams
  return) and then drains the HTTP server.
- `websocket.go` — `HandleConnect` (sends the login scene) and `HandleDisconnect` (marks the player
  disconnected in the game, closes the connection if we still own it). Despite the file name, both
  run for every transport.

### `internal/transport`

Everything above this package sees one `transport.Transport` (accepts connections, broadcasts, mounts
routes) and `transport.Conn` (one client: key/value state, `Write` for text, `WriteBinary` for bytes).

| Package | Wire protocol |
|---|---|
| `ws` | WebSocket via melody — the only file that imports melody. |
| `sse` | Server-Sent Events down, HTTP POST up, correlated by a connection ID. |
| `graphqlws` | `graphql-transport-ws` subprotocol over one WebSocket; framing only, no schema. |
| `webrtc` | Data channels via pion, signaled by one `POST /webrtc/offer`. |

`Composite` runs several at once as a single `Transport`; `injector/transport.go` builds it from
`INDRI_TRANSPORTS` and rejects unknown or duplicate names.

The non-melody transports share `QueuedConn` and `Hub` (`hub.go`), which enforce what callers rely on:

- **One writer per connection.** Writes queue; a single goroutine owns the socket/stream. A full
  queue returns `ErrBufferFull` instead of stalling a broadcast.
- **Serial delivery.** A connection's messages reach handlers one at a time, in order — `login`
  does check-then-set on connection state.
- **Exactly-once `Disconnect`, only after `Connect`**, with `IsClosed()` already true when it fires.
  `kick` closes a conn itself and the transport then reports the same close.
- **Queued writes flush on server close**, so a kick's `{"disconnected": true}` still arrives.
- **Connection IDs are bearer credentials** (256-bit, `crypto/rand`, never logged).

`origin.go` holds the origin allowlist and `Route`, which mounts an HTTP route behind it with CORS.

To add a transport: implement `Transport` (embedding `*Hub` gets you everything except `Register`),
pass `transporttest.Run` — the conformance suite every transport runs — and add a case to
`injector/transport.go`.

### `internal/handlers`

- `router/` — the registry and dispatcher. Handlers are a flat ordered slice; several may share an
  action and all matching ones run. Each invocation is wrapped in `recover()`.
- `actions/<name>/` — one package per action, each exposing `New(*injector.Injector)` and satisfying
  `actions.MessageHandler`.
- `utils/` — payload helpers (`DecodeMessageWithAction`, `RequireGameCode`, …).
- `messages/` — `WSMessage` interface plus a stub `auth.go` that is currently unused.

### `internal/services`

| Package | Responsibility |
|---|---|
| `boot` | Assemble the injector, register built-in handlers, run and shut down the two serving goroutines. |
| `mutation` | Generic serialized read-modify-write with a version fence. Backend-agnostic. |
| `lock` | `Manager`/`Handle` interface; `InProcess` and `Redis` implementations. |
| `events` | `Publisher` interface (`InProcess` channel, `Redis` Pub/Sub on `indri:changes`), the `ChangeEvent` wire type, `ClientView` (the game as clients hold it; shared by keyframes and deltas), `Diff`/`ToMap`, positional path encoding, and `SanitizeDelta`. |
| `game` | Game lifecycle, random code generation, `Sanitize`, player connect/disconnect. |
| `stage` | Scene CRUD, scene ordering, loading scenes from the script. |
| `broadcast` | Fan-out to a game, a team, specific players, or everyone. Targeted sends resolve recipients via the session store, then filter connections on the `sessionId` key. |
| `connection` | Thin wrapper over one `transport.Conn`: read/write, typed key access. |
| `authentication` | Password check + session issuance (bcrypt via `services/utils/password.go`). |
| `session`, `user` | Repo-backed lookups and updates. |
| `utils` | `HashPassword`/`CheckPasswordHash`, `GenerateToken`. |

### `internal/repo`

Game, user and session stores (one implementation per [database backend](#database-backends)) plus `env` (envconfig-backed, `INDRI_` prefix, cached singleton) and `script`
(reads and unmarshals the JSON script file once at boot).

`repo/game` is split by concern: `game.go` (CRUD, `Mutate`, `saveWithVersion`), `player.go`, `team.go`,
`host.go`. Each repo package's `interface.go` declares a `Storer` interface with a
`var _ Storer = (*Store)(nil)` compile-time assertion; this exists only as a drift guard (change a
`Store` method and the build breaks unless the interface is updated). Nothing consumes the
interfaces as an abstraction yet — callers use the concrete `*Store` types.

## Concurrency

Every write to a game goes through a store's `Mutate`, which delegates to `mutation.Run`:

1. Acquire a lock on `"game:<id>"`.
2. Load the game and its `version`, and snapshot its JSON form for the later diff.
3. Run the caller's `apply(*models.Game)` in memory (`mutation.ErrAbort` skips the write).
4. Save only if the stored version still equals `expected` (Mongo: `UpdateOne` filtered on
   `{_id, version}`), setting `version: expected+1`. If nothing matched, someone raced us — loop.
5. On a committed save, report the snapshot and saved game to the shared `changePublisher`
   (`repo/game/changes.go`), which diffs their client views and publishes a positional delta, or a
   keyframe request if the write changed the view's shape.
6. After 10 failed attempts, return `mutation.ErrConflict`.

The lock avoids the retry loop in the common case; the version fence is what makes a lease expiring
mid-mutation safe. Single-field writes (`UpdateField`, `DeleteField`, connect/disconnect) are shared
operations over `Mutate` (`repo/game/operations.go`), so every store changes and publishes games
identically; a store supplies only load, a version-fenced save, and queries.

`InProcess` locks and the in-process event bus are correct for a single instance only. Multi-instance
deployments must set `INDRI_LOCK_BACKEND=redis`, which switches both to Redis and enables the delivery
relay.

## Database backends

`INDRI_DB_BACKEND` picks the implementation behind the `game`, `user` and `session` `Storer`
interfaces; `injector/repos.go` switches on it.

| Value | Files | Notes |
|---|---|---|
| `mongodb` (default) | `repo/*/mongo*.go` | Client opened in `clients.go` from `INDRI_MONGO_*`. |
| `memory` | `repo/*/memory*.go` | In-process maps; lost on restart and not shared between instances. |
| `sqlite` | `repo/*/sqlite*.go` | One `*sql.DB` from `internal/clients/sqlite` (`INDRI_SQLITE_PATH`), pinned to a single connection; `Open` creates the schema. |
| `postgres` | `repo/*/postgres*.go` | One `*sql.DB` from `internal/clients/postgres` (`INDRI_POSTGRES_URI`), capped at 25 connections; each store runs its own `CREATE TABLE IF NOT EXISTS` DDL. |

The SQL backends store each record as a JSON blob (JSONB on Postgres) beside the columns needed for
lookups and uniqueness: game `code`/`version`/`private`, user `email`/`password`, session
`token`/`user_id`. Game writes go through the same `mutation.Run` lock and version fence as Mongo, with
the expected version in the `UPDATE ... WHERE`. The shared pool is `ReposInjector.SQLDB`, closed by
`closeResources` on shutdown.

Sessions expire `sessionMaxAge` (7 days) after creation on `mongodb` (TTL index) and `postgres`
(reads ignore older rows; `New` purges them). `memory` and `sqlite` do not expire sessions.

## Data model

Collections (tables `games`, `users`, `sessions` on the SQL backends): `game`, `user`, `session`.

```
Game
├── Code                 unique index
├── Version              optimistic-concurrency counter
├── Players  map[userId] → Player  { name, score, connected, host, controller, data, privateData }
├── Teams    map[teamId] → Team    { name, playerIds, data, privateData, playerData }
├── Stage
│   ├── CurrentScene, SceneOrder
│   ├── Scenes map[sceneId] → Scene { data, privateData, playerData }
│   └── data / privateData / playerData
└── data / privateData / playerData
```

Three data stores repeat at every level:

| Field | JSON/BSON | Visibility |
|---|---|---|
| `PublicData` | `data` | broadcast to everyone in the game |
| `PrivateData` | `privateData` | server-only; removed from keyframes and deltas alike by `events.ClientView` |
| `PlayerData` | `playerData` | keyed per player |

`Session` links a `userId` to a `gameId` and `teamId`, and holds the secret resume `Token`. Unique
indexes on `userId` and (sparse) `token`; compound indexes on `(userId, gameId)` and
`(gameId, userId, teamId)`.

A `Script` (`models.Script`, loaded from `config.json`) is the template new games are stamped from:
`Config` (pvp, maxTeams, maxPlayersPerTeam, profanityFilter, createTeams), initial `Teams`, initial
`Stage`, and initial data stores.

## Client (`client/`)

Expo Router / React Native app, three context providers (`game-state`, `user-state`, `game-list`), each
a reducer + provider + hook triple.

`services/message-handler.ts` owns the socket and a table of `{name, action, parser}` entries — the same
registry idea as the Go router, and extensible the same way by passing extra parsers to the constructor.

`services/game-state-parser.ts` is the delta engine: it stores a keyframe plus timestamp-sorted deltas,
rebuilds from a fresh clone on every reapply, drops deltas older than the keyframe cutoff, holds deltas
that arrive before any keyframe, and refuses to write through `__proto__`/`constructor`/`prototype` in
server-supplied dot paths.

## Layout authoring

The layout authoring feature lets a game host mutate `game.PublicData["layout"]` at runtime
through the `layout` WebSocket action. The full op vocabulary, authorization matrix, and
structural constraints are in `docs/PROTOCOL.md § layout`.

### Server side (`internal/handlers/actions/layout/`)

| File | Role |
|---|---|
| `op.go` | Op struct, typed sentinel errors, `DecodeOp`, `opRequired` table, `isMissing` |
| `validate.go` | `ValidateLayout` — size cap, privateData guard, grid dims, overlap/bounds, sub-grid depth |
| `handler.go` | Auth chain → `GameRepo.Mutate` → `applyLayoutOp` → `ValidateLayout` |

`ValidateLayout` is called inside `Mutate`'s apply function, so a validation failure rolls
back atomically with no write and no delta. It mirrors the TypeScript `parseLayout` validator
in `client/layout/schema/layout.ts` — **both must be changed together** whenever the rules
change.

Structural limits: `minDim=8`, `maxDim=4096`, `maxDepth=4` (sub-grid), `maxWidgets=300`,
`maxBytes=256 KiB`.

### Client side (`client/layout/edit/`, `client/components/board/editor/`)

| Module | Role |
|---|---|
| `edit/ops.ts` | `Sender` interface; op builder functions for all 7 ops; `moveWidget` runs `canPlace()` before sending |
| `edit/place.ts` | `firstFree` — row-major first-fit, bounded to 64×64 to avoid O(n³) scan on large grids |
| `edit/picker-helpers.ts` | Pure parse/format helpers: `parseColor`, `clampNumber`, `parseDate`, `dedupeValues` |
| `editor/drag-resize.tsx` | `DragResize` (Pan gesture, one op per gesture end, snap-back on reject), `GridOverlay` (suppressed above 4096 cells) |
| `editor/palette.tsx` | `Palette` (chip list from registry), `RemoveButton` |
| `editor/config-panel.tsx` | `ConfigPanel` — descriptor → picker via `PICKERS` mapped type; validates against widget schema before emitting |
| `editor/pickers/*.tsx` | Eight hand-rolled cross-platform pickers; text/number/color/date/uri debounce at 250 ms |

`GestureHandlerRootView` wraps the app root in `client/app/_layout.tsx` (required on every
platform including web). The edit-mode toggle lives in `client/app/board/index.tsx`.

## Configuration

`internal/repo/env/env.go`, all variables prefixed `INDRI_`, documented in `.env.example`. Highlights:

| Variable | Default | Note |
|---|---|---|
| `INDRI_LISTEN_ADDRESS` / `INDRI_LISTEN_PORT` | `localhost` / `5002` | |
| `INDRI_ALLOWED_ORIGINS` | `""` | Comma-separated. Empty rejects all cross-origin browsers. |
| `INDRI_DB_BACKEND` | `mongodb` | `mongodb`, `memory`, `sqlite` or `postgres`. |
| `INDRI_SQLITE_PATH` / `INDRI_POSTGRES_URI` | `./indri.db` / `""` | Used by the `sqlite` / `postgres` backends. Pass the Postgres URI by env var, not the `-postgres-uri` flag, since flags are visible in `ps`. |
| `INDRI_MONGO_URI` / `INDRI_MONGO_DATABASE` | `localhost` / `indri` | A replica set is not required. |
| `INDRI_LOCK_BACKEND` | `inprocess` | Multi-instance switch: `redis` moves both the lock manager and the event bus to Redis. |
| `INDRI_REDIS_*` | localhost:6379 | Only used in `redis` mode. |
| `INDRI_WS_*` | see `.env.example` | Write timeout, ping period, pong timeout, max message size, buffer size. |

## Testing

`go test -race ./...` in CI. Tests that need MongoDB (`internal/repo/game/game_concurrency_test.go`)
call `t.Skipf` when no database is reachable, so CI stays green without one. Point them elsewhere with
`INDRI_TEST_MONGO_URI`; they use the `indri_test` database. The Postgres store tests skip unless
`INDRI_TEST_POSTGRES_URI` is set (CI runs a `postgres:15-alpine` service for them); they `TRUNCATE`
their tables in whatever database it points at.
