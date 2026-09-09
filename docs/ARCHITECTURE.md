# Indri architecture

A component map with file references. For the wire format see [PROTOCOL.md](PROTOCOL.md); for
day-to-day commands and conventions see [../CLAUDE.md](../CLAUDE.md).

## Request lifecycle

```
browser / native client
        │  WebSocket /ws   ·or·   GraphQL /graphql (mutations + subscription)
        ▼
internal/entrypoints/http/server.go      net/http mux, timeouts, graceful shutdown
        ▼
internal/transport/                      Transport interface: ws (melody) + graphql (gqlgen),
                                         aggregated by transport.Multi. Resolves the session, then:
        ▼
internal/handlers/router/                DispatchMessage (WS) / Dispatch (GraphQL): decode, run
                                         "received" → <action> → "processed"; recover() per handler
        ▼
internal/handlers/actions/<action>/      one package per action — Handle(Request) (Result, error)
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
        └────── internal/services/broadcast/         Transport.BroadcastFilter → WS + GraphQL clients
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
| `clients.go` | Mongo client, the WebSocket transport, lock manager, change-event publisher. Cached process-globally; all injectable for tests. |
| `repos.go` | `game`, `user`, `session` Mongo stores (each creates its indexes in `NewStore`) plus the script store. |
| `services.go` | game, broadcast, auth, user, session services. Also builds the GraphQL transport (which needs the session store) and wraps it with the WebSocket transport in a `transport.Multi`, which becomes the injector's `Transport`. |

`injector.Injector` embeds all three structs plus `GlobalContext` and the parsed `Script`, and is the
single value threaded into every handler.

`INDRI_LOCK_BACKEND` is really a multi-instance switch. Set to `redis`, `GetClients` opens one Redis
connection and shares it between `lock.Redis` and `events.Redis`; otherwise both the lock manager and
the event bus are in-process.

### `internal/transport`

The client wire protocol is behind two interfaces so it can be swapped without touching handlers,
services, or routing:

- `Conn` — one client connection: `Get/Set/UnSet(key)`, `Write`, `Close`, `IsClosed`.
- `Transport` — the hub: `Handle(Handlers{Connect,Disconnect,Message,Error})`, `Register(mux)` (each
  transport mounts its own routes), `Broadcast`, `BroadcastFilter`, `Conns`, `Close`, `IsClosed`.

| Adapter | Route | Notes |
|---|---|---|
| `ws` | `/ws` | melody-backed WebSocket. The **only** package that imports melody. Origin check, ping/pong, size limits. |
| `graphql` | `/graphql` | gqlgen. Typed mutations → `router.Dispatch` (auth from the `Authorization` header); a `gameUpdates` subscription (graphql-transport-ws, auth from `connection_init`) whose push conn is a channel-backed `Conn` fed by the existing broadcast. Dynamic data uses a `JSON` scalar (`json.RawMessage`). |
| `Multi` | — | Aggregates the above so `broadcast` fans out to both; `Conns` are unioned. Adding a protocol (REST+SSE, WebRTC, WebTransport) means a new adapter, nothing above the interface. |

### `internal/entrypoints`

- `http/server.go` — mounts every transport's routes (`Transport.Register(mux)`) and runs the HTTP
  server with read/write/idle timeouts. On context cancellation it closes the transport *first* (so
  hijacked connections return) and then drains the HTTP server.
- `websocket.go` — `HandleConnect` (sends the login scene) and `HandleDisconnect` (marks the player
  disconnected in the game, closes the connection if we still own it). Both are transport-agnostic
  (`transport.Conn`).

### `internal/handlers`

Handlers are **connection-independent**: `Handle(actions.Request{Session, Payload}) (actions.Result,
error)`. The transport resolves the authenticated session and applies the `Result` (write `Responses`,
bind `Session` on auth, force-close `DisconnectIDs`). This is what lets WebSocket messages and GraphQL
mutations share one code path.

- `router/` — `Dispatch(session, action, payload)` runs the registry (`received` → action →
  `processed`; `recover()` per handler); `DispatchMessage` decodes a raw frame for message transports.
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
| `events` | `Publisher` interface (`InProcess` channel, `Redis` Pub/Sub on `indri:changes`), the `ChangeEvent` wire type, `Diff`/`ToMap` for computing dotted-path deltas from JSON representations, and `SanitizeDelta` for stripping private data out of them. |
| `game` | Game lifecycle, random code generation, `Sanitize`, player connect/disconnect. |
| `stage` | Scene CRUD, scene ordering, loading scenes from the script. |
| `broadcast` | Fan-out to a game, a team, specific players, or everyone via `Transport.BroadcastFilter`. Targeted sends resolve recipients via the session store, then filter connections on the `sessionId` key — so WS and GraphQL subscribers are reached identically. |
| `connection` | Thin wrapper over one `transport.Conn`: read/write, typed key access, cross-connection lookup. |
| `authentication` | Password check + session issuance (bcrypt via `services/utils/password.go`). |
| `session`, `user` | Repo-backed lookups and updates. |
| `utils` | `HashPassword`/`CheckPasswordHash`, `GenerateToken`. |

### `internal/repo`

Mongo stores plus `env` (envconfig-backed, `INDRI_` prefix, cached singleton) and `script`
(reads and unmarshals the JSON script file once at boot).

`repo/game` is split by concern: `game.go` (CRUD, `Mutate`, `saveWithVersion`), `player.go`, `team.go`,
`host.go`. Each repo package's `interface.go` declares a `Storer` interface with a
`var _ Storer = (*Store)(nil)` compile-time assertion; this exists only as a drift guard (change a
`Store` method and the build breaks unless the interface is updated). Nothing consumes the
interfaces as an abstraction yet — callers use the concrete `*Store` types.

## Concurrency

Every multi-field edit to a game goes through `gameRepo.Store.Mutate`, which delegates to
`mutation.Run`:

1. Acquire a lock on `"game:<id>"`.
2. Load the game and its `version`, and snapshot its JSON form for the later diff.
3. Run the caller's `apply(*models.Game)` in memory (`mutation.ErrAbort` skips the write).
4. `UpdateOne` filtered on `{_id, version: expected}` with `$set` of the whole document and
   `version: expected+1`. If `MatchedCount == 0`, someone raced us — loop.
5. On a committed save, diff the snapshot against the saved game and publish the delta.
6. After 10 failed attempts, return `mutation.ErrConflict`.

The lock avoids the retry loop in the common case; the version fence is what makes a lease expiring
mid-mutation safe. Single-field writes (`UpdateField`, `DeleteField`, `markPlayerConnected`) skip the
lock but still `$inc` `version`, keeping them coherent with the CAS, and publish their own delta
explicitly.

`InProcess` locks and the in-process event bus are correct for a single instance only. Multi-instance
deployments must set `INDRI_LOCK_BACKEND=redis`, which switches both to Redis.

## Data model

Collections: `game`, `user`, `session`.

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
| `PrivateData` | `privateData` | server-only; removed from keyframes by `GameService.Sanitize` and from deltas by `events.SanitizeDelta` |
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

## Configuration

`internal/repo/env/env.go`, all variables prefixed `INDRI_`, documented in `.env.example`. Highlights:

| Variable | Default | Note |
|---|---|---|
| `INDRI_LISTEN_ADDRESS` / `INDRI_LISTEN_PORT` | `localhost` / `5002` | |
| `INDRI_ALLOWED_ORIGINS` | `""` | Comma-separated. Empty rejects all cross-origin browsers. |
| `INDRI_MONGO_URI` / `INDRI_MONGO_DATABASE` | `localhost` / `indri` | A replica set is not required. |
| `INDRI_LOCK_BACKEND` | `inprocess` | Multi-instance switch: `redis` moves both the lock manager and the event bus to Redis. |
| `INDRI_REDIS_*` | localhost:6379 | Only used in `redis` mode. |
| `INDRI_WS_*` | see `.env.example` | Write timeout, ping period, pong timeout, max message size, buffer size. |

## Testing

`go test -race ./...` in CI. Tests that need MongoDB (`internal/repo/game/game_concurrency_test.go`)
call `t.Skipf` when no database is reachable, so CI stays green without one. Point them elsewhere with
`INDRI_TEST_MONGO_URI`; they use the `indri_test` database.
