# Indri architecture

A component map with file references. For the wire format see [PROTOCOL.md](PROTOCOL.md); for
day-to-day commands and conventions see [../CLAUDE.md](../CLAUDE.md).

## Request lifecycle

```
browser / native client
        │  WebSocket /ws · GraphQL /graphql · REST /api/<action> + SSE /events · WebRTC /rtc/offer
        ▼
internal/entrypoints/http/server.go      net/http mux, timeouts, one shared OriginPolicy
                                         (allowlist + CORS), graceful shutdown
        ▼
internal/transport/                      Transport interface: ws (melody), graphql (gqlgen),
                                         sse, rest, webrtc (pion) — aggregated by
                                         transport.Multi. Resolves the session, then:
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
        └────── internal/services/broadcast/         BroadcastFilter → WS, GraphQL, SSE, WebRTC
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
| `services.go` | game, broadcast, auth, user, session services. Also builds the GraphQL, SSE and REST transports (each needs the session store to authenticate bearer tokens) and wraps them with the WebSocket transport in a `transport.Multi`, which becomes the injector's `Transport`. Each is given the aggregate as its peer, so a kick closes the target's connections on every transport. |

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

Three pieces in the package are shared by the push-only transports, so GraphQL subscriptions and SSE
streams behave identically rather than approximately:

| Type | What it owns |
|---|---|
| `Keys` | Per-connection key/value state and the closed flag. `WhileOpen` holds the read lock for a delivery, so a send cannot overlap the `Close` that releases what it sends to; `MarkClosed` is won by exactly one caller. |
| `BufferedConn[T]` | A push-only `Conn` queueing onto a buffered channel. Drops the oldest rather than blocking the broadcaster on a slow client. Generic only so each transport hands out the channel type its framework wants. |
| `Registry` | The open-connection set plus fan-out — every part of `Transport` that is protocol-independent. A transport embeds it and supplies only `Handle` and `Register`. |
| `OriginPolicy` | The origin allowlist, same-origin acceptance, and the CORS middleware. Applied once in `entrypoints/http`, so all four routes agree. |

| Adapter | Route | Notes |
|---|---|---|
| `ws` | `/ws` | melody-backed WebSocket. The **only** package that imports melody. Origin check, ping/pong, size limits. |
| `graphql` | `/graphql` | gqlgen. Typed mutations → `router.Dispatch` (auth from the `Authorization` header); a `gameUpdates` subscription (graphql-transport-ws, auth from `connection_init`) whose push conn is a channel-backed `Conn` fed by the existing broadcast. Dynamic data uses a `JSON` scalar (`json.RawMessage`). |
| `sse` | `/events` | Server-Sent Events, push-only. Authenticated at the handshake from `Authorization` or a `token` query parameter — the browser `EventSource` API cannot set headers. Clears the server's `WriteTimeout` per stream (unlike a WebSocket upgrade, SSE never hijacks the connection), emits a `:ping` comment while idle, and frames each delta as `data:`. |
| `rest` | `/api/<action>` | The inbound half for SSE clients: one POST route per action, arguments validated then dispatched to `router.Dispatch` (auth from the `Authorization` header). Holds no connections — its `Registry` stays empty, so it contributes routes and nothing to fan-out. |
| `webrtc` | `/rtc/offer` + a DataChannel | pion. Bidirectional and message-oriented, so it implements `Handle` in full and is shaped like `ws`, not like the push-only adapters — the existing dispatch path drives it unchanged. One HTTP round trip performs non-trickle signalling; the `game` DataChannel becomes a `Conn`, `signal` is reserved for renegotiation and never reaches the router. Signalling is optionally authenticated, so a peer may start anonymous and bind its session on `login`. A global peer cap and a pending TTL bound what an unauthenticated caller can allocate. |
| `Multi` | — | Aggregates the above so `broadcast` fans out to all of them; `Conns` are unioned. Adding a protocol (WebTransport, …) means a new adapter, nothing above the interface. |

### `internal/entrypoints`

- `http/server.go` — mounts every transport's routes (`Transport.Register(mux)`), wraps the mux in the
  shared `OriginPolicy.Middleware`, and runs the HTTP server with read/write/idle timeouts. On context
  cancellation it closes the transport *first* (so hijacked connections and SSE streams return) and
  then drains the HTTP server.
- `websocket.go` — `HandleConnect` (sends the login scene) and `HandleDisconnect` (marks the player
  disconnected in the game, closes the connection if we still own it). Both are transport-agnostic
  (`transport.Conn`).

### `internal/handlers`

Handlers are **connection-independent**: `Handle(actions.Request{Session, Payload}) (actions.Result,
error)`. The transport resolves the authenticated session and applies the `Result` (write `Responses`,
bind `Session` on auth, force-close `DisconnectIDs`). This is what lets WebSocket messages and GraphQL
mutations, REST posts and SSE streams share one code path.

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

It also carries `schemaVersion` and the `scripts` list. `schemaVersion` is the config format the file
was written for; a version this build does not know fails boot, and a missing one is read as the
current version and warned about. Each `scripts` entry is `{"path": ..., "grants": [...]}` — the Lua
game script to load and the host capabilities it may reach. Grants are **per script**: a capability a
script was not granted is absent from the `indri` table it sees, not present behind a permission
check, so what a script can do is a glance at its grant list rather than an audit of every host
function. An unrecognised grant name fails boot, and a config with no `scripts` at all warns loudly,
because the server it produces answers nothing but the built-in actions
(`internal/services/lua/capability.go`, `internal/repo/script/script.go`).

## Layout engine

A **layout** is the board a game draws: a grid, a set of scenes, and the widgets placed in them. It is data,
not code, so a game changes its board without a rebuild.

### Where it lives

`game.data.layout` is the canonical location — **one object per game**, with a `scenes` map inside it keyed
by scene id. `stage.currentScene` selects which entry renders. Nothing is stored under `scene.data`. One
object gives the `layout` action a single subtree to authorize and to validate, and makes a scene switch a
pure lookup.

```
game.data.layout
├── grid            { cols, rows }          the board's coordinate space, 8..4096 each
├── style                                   board style, optional
├── script                                  board Lua script, optional
└── scenes  map[sceneId] → { style?, script?, widgets }
                            └── widgets map[widgetId] → { type, placement, config?, style?, script? }
```

**Widgets are a map keyed by id, never an array.** `events.Diff` replaces arrays whole, so an array would
re-send every widget on any single change and defeat the delta pipeline the engine is built on. Because
every level is a JSON object, `Diff` walks to the leaf and one cell update arrives as one tight path:

```
data.layout.scenes.board.widgets.cells.config.widgets.c01.config.text  =  "X"
```

### Server side — `internal/handlers/actions/layout`

One host-only action carries seven ops behind an `op` discriminator (see
[PROTOCOL.md](PROTOCOL.md#layout)). `op.go` decodes and shape-checks the op, `apply.go` applies it to a
detached copy of the layout inside `Store.Mutate`, and `validate.go` validates the **whole resulting
document** before the copy is assigned back. Validating the op alone could not work: only the result tells
you whether an overlap now exists. A rejected op leaves the game exactly as it was, and the write is confined
to `PublicData["layout"]` — `PrivateData`, `Players`, `Teams` and `Stage` are not reachable from there.

### Client side — pure core, thin renderer

The client is split in two, and the boundary is enforced by what each half may import.

| Path | Contains |
|---|---|
| `client/layout/` | Pure TypeScript, no React Native imports: schema, grid geometry, collision, style compile, the widget registry, and the Lua host. Runs in bare Node, which is what makes the feature testable at all — the client has no React test renderer. |
| `client/components/board/` | React Native components only. They render what the core computes and hold no layout logic. |

`parseLayout` never throws. It runs against wire data on every keyframe, so it degrades to an issue list
instead. Bad geometry is clamped, but a schema violation is an error-severity issue and the parse returns no
layout at all.

### Lua is the binding layer

There is no declarative binding language. A script does the mapping from game state to widgets, so a second
expression evaluator over the same state would be redundant surface area.

Scripts are **asymmetric**: they send actions outbound through `indri.send(…)`, and receive nothing inbound.
A script learns about the world only by observing the reduced game state through the `stateChanged` event. A
script that read raw messages could interpret one differently from the reducer and desynchronise from
authoritative state, so there is deliberately no `message` event.

A script never writes game state. Presentation writes land in a local **override layer**
(`client/layout/lua/overrides.ts`), which `mergeOverrides` composites over the server layout at render time.
The delta stream and the scripts therefore never touch the same object. Overrides are cleared on a keyframe
and only on a keyframe: a keyframe is a resync, while a delta is incremental and must not wipe a script's
work.

### Two validators, on purpose

`internal/handlers/actions/layout/validate.go` and the TypeScript schema under `client/layout/schema/` are a
**deliberately duplicated pair**. They cannot share code across languages, so the rules are written twice and
the constants and the AABB overlap test are kept textually identical. Every Go rule names the file it
mirrors. Change one side and you must change the other.

The two halves are not interchangeable. The client's `parseLayout` is UX feedback: it clamps and warns so a
sloppy board still renders. The Go validator is the security boundary: it rejects rather than repairs,
because a rule that only the client enforces is not enforced at all.

## Host-authored content — accepted risk

**Read this before any public deployment.**

A host can set arbitrary media URIs and arbitrary Lua source in their game's layout. The server delivers both
to every player in that game, where the client loads the URI and executes the script. **There is no URI
allow-listing and no script review.** This is an accepted risk for a proof of concept. Do not ship it to a
public deployment without both.

The caps that do exist are structural only. They bound the cost of a layout; they do not judge its content.

| Cap | Value | Source |
|---|---|---|
| Widgets per layout | 300 | `maxWidgets` — counted across all scenes and all nested sub-grids |
| Serialised layout size | 256 KiB | `maxBytes` |
| Sub-grid nesting | 4 grid levels | `maxDepth` — the scene's own grid is level 1, so at most three nested sub-grids |
| Grid dimension (`cols`, `rows`) | 8 to 4096 | `minDim`, `maxDim` |

All four are in `internal/handlers/actions/layout/validate.go`.

**Script source length is not capped directly.** `maxBytes` bounds it only transitively, as part of the
serialised layout. A single script can therefore be almost 256 KiB of text that every client will execute.
The client runs scripts under an instruction budget (`DEFAULT_INSTRUCTION_BUDGET`, 200 000 VM instructions
per top-level call), but that budget is best-effort: it counts VM instructions, and one instruction can do
unbounded work.

**Widget `config` and `style` contents are opaque to the server by design.** Indri is a framework and does
not know what a "text" widget is, so encoding widget semantics server-side would duplicate the client
registry and go stale. The consequence is worth stating: a bad `style` key passes the server, and the
client's strict parse then rejects the **whole** layout. Every player in that game sees "This board could not
be loaded." instead of any board at all. One typo in one widget takes down the board for everyone.

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
| `INDRI_ALLOWED_ORIGINS` | `""` | Comma-separated, matched as exact strings (host *and* port; `localhost` ≠ `127.0.0.1`). Empty rejects all cross-origin browsers. |
| `INDRI_MONGO_URI` / `INDRI_MONGO_DATABASE` | `localhost` / `indri` | A replica set is not required. |
| `INDRI_LOCK_BACKEND` | `inprocess` | Multi-instance switch: `redis` moves both the lock manager and the event bus to Redis. |
| `INDRI_REDIS_*` | localhost:6379 | Only used in `redis` mode. |
| `INDRI_WS_*` | see `.env.example` | Write timeout, ping period, pong timeout, max message size, buffer size. |

## Testing

`go test -race ./...` in CI. Tests that need MongoDB (`internal/repo/game/game_concurrency_test.go`)
call `t.Skipf` when no database is reachable, so CI stays green without one. Point them elsewhere with
`INDRI_TEST_MONGO_URI`; they use the `indri_test` database.
