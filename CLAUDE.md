# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

Indri is a Go backend for real-time, multiplayer, browser/mobile party games. Clients talk to it using
JSON messages routed by an `action` field, over WebSocket (`/ws`, the default) or any other transport
enabled in `INDRI_TRANSPORTS` (SSE, GraphQL subscriptions, WebRTC — see `docs/PROTOCOL.md`). Game state lives in
the database chosen by `INDRI_DB_BACKEND` (MongoDB by default; also memory, SQLite, PostgreSQL); each
write computes its own delta in application code and publishes it on an event bus, which broadcasts it
to everyone in that game. `client/` is a companion Expo/React Native reference client.

Indri is a *framework*: the generic actions (register/login/join/create/…) ship in `internal/`, and a
concrete game adds its own action handlers plus a JSON "script". `example/tictactoe/` is the worked
example of that pattern.

## Commands

```bash
# Go server
go build ./...
go vet ./...
go test ./...
go test -race ./...                                   # what CI runs
go test ./internal/services/mutation/                 # one package
go test -run TestAddPlayer_ConcurrentNoLostUpdates ./internal/repo/game/   # one test

go run ./cmd/server -script ./config.json             # run (-script is required)
go run ./example/tictactoe                            # tic-tac-toe example (-script defaults to ./config.json)
go run ./cmd/server -script ./config.json -h          # every settings flag; both binaries accept them
air                                                    # hot reload (.air.toml)

# Infrastructure (MongoDB + Redis) — requires a populated .env
docker compose up -d

# Client (client/, pnpm only — there is no package-lock.json)
pnpm install
pnpm run typecheck
pnpm test          # node --experimental-strip-types on services/game-state-parser.node-test.ts
pnpm run lint
pnpm run web       # or: start / ios / android (Expo Go)
npx expo run:ios   # development build: needed for EXPO_PUBLIC_TRANSPORT=webrtc on native; ios/ + android/ are generated, gitignored
```

Config is environment-driven with an `INDRI_` prefix (`internal/repo/env/env.go`); `.env.example`
documents every variable. Copy it to `.env` before running anything.

MongoDB does **not** need to be a replica set — deltas are computed by the application, not read from a
change stream. `docker-compose.yml` still starts one (`rs0`, initiated in its healthcheck), which is
harmless.

## Architecture

### Boot chain

`cmd/server/main.go` → `cli.Parse` → `boot.Boot(ctx, scriptPath)` → `boot.Serve(i)`.

`internal/cli` owns the command line for every server binary (the example included): `-script`,
`-config` (default `server.json` beside the script), and one flag per `env.Vars` field with a `flag`
tag. Settings resolve JSON config < env vars < flags. Add a setting's flag there, or
`TestParse_RegistersAFlagForEverySetting` fails.

`Boot` wires a three-layer injector (`internal/injector/`), in strict order:

1. **clients** (`clients.go`) — MongoDB (only for the `mongodb` backend), the client transport (built
   from `INDRI_TRANSPORTS` by `transport.go`), the lock manager, and the change-event publisher. Clients are process-global singletons; `GetClients` returns a cached `*ClientsInjector` on
   every call after the first, and accepts overrides for tests. `INDRI_LOCK_BACKEND=redis` is the
   *multi-instance switch*: it flips **both** the lock manager and the event bus to Redis, sharing one
   connection. Otherwise both are in-process.
2. **repos** (`repos.go`) — game/user/session stores for `INDRI_DB_BACKEND` (`mongodb` default, `memory`,
   `sqlite`, `postgres`; see "Database backends" in `docs/ARCHITECTURE.md`) plus the script store (loaded
   from the JSON file). The SQL backends share one `*sql.DB` (`ReposInjector.SQLDB`), closed on shutdown.
3. **services** (`services.go`) — business logic over the repos.

`*injector.Injector` embeds all three, so handlers reach anything via one struct (`h.i.GameService`,
`h.i.GameRepo`, `h.i.Transport`, …). Every handler takes it in `New(i *injector.Injector)`.

`Serve` runs two goroutines under an `errgroup`: the HTTP server and the change-stream
broadcaster. Both shut down on root-context cancellation (SIGINT/SIGTERM), then `closeResources` drains
the transport and the Mongo or SQL pool.

### Transports

Nothing above `internal/transport/` knows the wire protocol: handlers get a `transport.Conn`, services
broadcast through `transport.Transport`. `ws` (melody — the only melody import), `sse`, `graphqlws`,
and `webrtc` implement it; `transport.Composite` runs several at once as one. Every transport must
pass `transporttest.Run`, the shared conformance suite — it pins down what callers depend on (serial
message delivery per connection, exactly-once `Disconnect` with `IsClosed()` already true, queued
writes flushed on a server-side close). See `docs/ARCHITECTURE.md` for the invariants and how to add
one. `Conn.Write` is text; `Conn.WriteBinary` is for opaque bytes such as MessagePack.

### Inbound: message routing

`Transport.Handle` → `router.HandleMessage` (`internal/handlers/router/`):

1. `utils.DecodeMessageWithAction` unmarshals to `map[string]interface{}`, pulls out `action`, and
   **deletes `action` from the payload** before handing it on.
2. `router.Act` runs three pseudo-phases per message: **`received` → `<action>` → `processed`**. Any
   handler registered under the literal actions `received` or `processed` therefore runs on *every*
   message — that is the intended pre/post hook mechanism. Nothing registers them by default.
3. `invokeHandler` wraps each call in a `recover()`, converting a panic into an error so a malformed
   client message can't kill the process or leak the connection.

Handlers are a flat, ordered `[]Handler` registry (`register.go`) — multiple handlers may share one
action, and all matching handlers run. `boot.registerHandlers` installs the built-ins; a game adds its
own with `router.RegisterHandler(name, action, handler)` **after** `boot.Boot` and before `boot.Serve`
(see `example/tictactoe/main.go`).

### Outbound: keyframes and deltas

Two different shapes reach the client:

- **Keyframe** — the game's *client view* (`events.ClientView`: private data removed at every depth,
  `data.layout` removed because it is sent once in a layout frame) with a schema version `sv` (the layout
  hash). Written to the requesting connection on join/create/refresh/reconnect, and broadcast to a whole
  game when a write changes its shape (below).
- **Delta** — an `events.ChangeEvent` (`{o, t, u, r}`) broadcast to every session in the affected game.
  Paths are **positional**: each object key is replaced by its index among its sorted siblings in the
  client view, so a client decodes them against the keyframe it holds.

Keyframes and deltas share one definition of what the client holds: both are built from
`events.ClientView`. Change what clients may see there, and nowhere else.

The delta path is **application-computed, not database-driven** (`internal/services/events`; there is no
MongoDB change stream):

```
Store.Mutate commit ──► changePublisher.diff (repo/game/changes.go) ──► Publisher.Publish
                          ClientView(before/after), Diff,                 │
                          positional encode                               │
                                         in-process channel ──────────────┤
                                         or Redis Pub/Sub  ───────────────┘
                                                                          ▼
                                         boot.monitorGameChanges (Subscribe)
                                                                          ▼
                                         BroadcastService.Broadcast(gameID, …)
```

**Shape changes re-keyframe.** A write that adds or removes an object key in the client view shifts
positions the client's schema can't know, so instead of a delta `changePublisher` publishes an
`OpKeyframe` request and the broadcaster sends that game a fresh keyframe. Value changes and array
growth/shrink stay deltas. Pre-declared player slots and `null` placeholders (e.g. `winningTeam`) exist to
keep ordinary play shape-stable, so avoid adding and removing keys during a game where a value will do.

The client (`client/services/game-state-parser.ts`) rebuilds state by cloning the last keyframe and
replaying timestamp-ordered deltas over it, discarding deltas older than the keyframe's cutoff.

**A write that never publishes is invisible to players.** Every game write now goes through `Mutate`, and
every store reports each committed `Mutate` to the one shared `changePublisher`, so a new write path gets
publishing for free as long as it is built on `Mutate`:

- `UpdateField`, `DeleteField`, and connect/disconnect are shared operations over `Mutate`
  (`repo/game/operations.go`), not per-store partial updates. A write that changes nothing clients can
  see publishes nothing.
- Publish failures are logged, never returned — the write already committed, so a fan-out hiccup must
  not fail the mutation.

`BroadcastService` offers game, team, player, and global fan-out. Because `sessionId` is the only
per-connection key the app sets, every targeted send resolves its recipients through the session store
first and then filters connections on that one key (`broadcastToSessions`). Do not invent new
connection keys to filter on — nothing sets them.

### Concurrency model

Game documents are edited by many connections at once, so `internal/services/mutation` owns
conflict resolution *above* the data store:

```
mutation.Run(ctx, lockManager, key, load, apply, save)
  └─ acquire distributed lock  →  loop: load(doc, version) → apply(doc) → save(doc, expectedVersion)
```

- `internal/services/lock` is the mutual-exclusion interface. `InProcess` (keyed mutexes) is the default;
  set `INDRI_LOCK_BACKEND=redis` for multi-instance deployments (`lock.Redis`, SET NX + per-holder token,
  10s lease, token-checked Lua release).
- The lock makes losing races rare; the **version fence** makes them safe. `models.Game.Version` is
  compared in the update filter and incremented on commit, so a lease that expires mid-mutation cannot
  cause a lost update. `mutation.Run` retries up to 10 times, then returns `ErrConflict`.
- `apply` may return `mutation.ErrAbort` to signal "no change needed" and skip the write.
- Each game store (`MongoStore`, `MemoryStore`, `SQLiteStore`, `PostgresStore`) binds `Mutate` to its
  own load and version-conditional save. **Use `Mutate` for any read-modify-write on a game.** Game
  logic shared by every store — slot assignment, removal, connect/disconnect, field writes, building a
  new game from the script — lives once in `internal/repo/game/operations.go` over `Mutate`, and
  `store_contract_test.go` runs the same player tests against every available backend.

Nothing here is Mongo-specific by design: a different backend only needs to supply `load` and a
version-conditional `save`.

### Sessions and identity — the `sessionId` overload

There are **two different values both called `sessionId`**, and confusing them is a security bug:

| Where | Value | Notes |
|---|---|---|
| Connection key `sessionId` | `session.ID.Hex()` (Mongo ObjectID) | Server-side targeting key for broadcasts. Never sent to clients. |
| JSON field `sessionId` on the wire | `session.Token` (256-bit hex) | Unguessable bearer token. Client echoes it back in a `reconnect`. |

`login` and `reconnect` are the only places that call `SetKey`, and the only key they set is `sessionId`.
Authorization must always resolve the caller from *their own connection's* `sessionId`, never from a
client-supplied `userId` — `kick` is the reference implementation.

Sessions are one-per-user: `sessionRepo.New` returns the existing session for a `userId` rather than
creating a second one (unique index on `userId`).

### Data model

Collections: `game`, `user`, `session` (tables `games`, `users`, `sessions` on the SQL backends). Models
live in `internal/models/`.

A **Game** holds `Teams`, `Players`, and a `Stage`. A **Stage** holds ordered **Scenes** plus a
`currentScene`; a scene is the visual unit the client renders. Games, stages, scenes, teams, and players
each carry up to three data stores:

- `PublicData` (`data`) — broadcast to everyone in the game.
- `PrivateData` (`privateData`) — server-only; removed from everything sent to clients by `events.ClientView`.
- `PlayerData` (`playerData`) — keyed per player.

A **Script** (`config.json`, `models.Script`) is the template a new game is stamped from: config, initial
teams, and the initial stage/scenes. It is loaded once at boot from `-script` and exposed as
`i.Script`.

## Adding a game action

1. Create `example/<game>/server/handlers/<action>/handler.go` with a `Handler` struct holding
   `*injector.Injector`, a `New(i)` constructor, and
   `Handle(s transport.Conn, decodedMsg map[string]interface{}) error`.
2. Resolve the caller: `connection.NewService(s, h.i.Transport).GetKeyAsString("sessionId")` →
   `h.i.SessionService.Get(...)` → `GetGameIDAndTeamID(...)`.
3. Validate the move against the current game state, then write through `h.i.GameRepo.Mutate` (or
   `UpdateField` for a single independent field). Do **not** write the response yourself for state
   changes — the published delta broadcasts it.
4. Register it in `main.go` after `boot.Boot`: `router.RegisterHandler("<game>_<action>", "<action>", <pkg>.New(i))`.

Built-in actions register in `boot.registerHandlers` instead, and
`TestRegisterHandlers_CoversEveryActionPackage` fails if a package under `internal/handlers/actions/`
is never wired up — an unregistered action is silently unreachable, so the test exists to catch that.

## Conventions

- Errors: wrap with `%w` and lowercase context (`fmt.Errorf("fetching game %q: %w", id, err)`).
- Logging is stdlib `log` throughout; there is no structured logger.
- Handler/service/repo constructors validate their dependencies and return an error rather than panicking.
- Client-facing protocol errors are `models.WSError` values (`internal/models/errors.go`) written with
  `connection.Service.WriteError`.
- Import grouping is stdlib / third-party / `github.com/robbiebyrd/indri/...`.
- Each `repo/*` package declares a `Storer` interface with a `var _ Storer = (*Store)(nil)` assertion.
  Add or change an exported `Store` method and you must update `interface.go` too, or the build breaks.
- The client uses pnpm and has no test runner beyond `node --experimental-strip-types` on
  `*.node-test.ts` files.
- A green local `pnpm run typecheck` does not prove CI will pass. A working `client/node_modules` can
  have transitive packages hoisted to the top level, so an **undeclared** dependency still resolves
  locally while CI's clean `pnpm install --frozen-lockfile` fails on it. After touching client imports,
  verify the way CI does:
  `rm -rf node_modules && pnpm install --frozen-lockfile && pnpm run typecheck && pnpm test`.

## Layout authoring

`game.PublicData["layout"]` (JSON path `data.layout`) is the canonical location for all layout data.
It is written exclusively through the `layout` WebSocket action (`internal/handlers/actions/layout/`).

**`privateData` is a reserved key at any depth inside the layout document.** The server's
`ValidateLayout` rejects it unconditionally, and the TypeScript `parseLayout` function does the same.
Do not store anything under that key in a layout.

**The Go validator (`validate.go`) and the TypeScript validator (`client/layout/schema/layout.ts`) are
a deliberate duplicated pair.** They must be changed together whenever the structural rules change
(grid bounds, overlap rules, sub-grid depth cap, reserved keys). The Go validator enforces the rules
for writes; the TS validator enforces them for the client render path. Changing one without the other
creates a split-brain.

**Residual risk: host-supplied content.** The `setScript` op stores Lua source verbatim; the
`addWidget` op stores widget configs (including image URIs) verbatim. The server validates structure
only — it does not sandbox script execution or allow-list URIs. Before deploying to an environment
where hosts are untrusted:
- Restrict `setScript` at the authorization layer or audit all script content.
- Add URI allow-listing for image and video widgets.

## Known rough edges

Do not treat these as intentional; check before relying on them.

- `docs/tasks.md` is an aspirational backlog, not a description of current state. Several of its
  entries (dependency injection, context propagation, CI) are already done.

## Reference docs

- `docs/ARCHITECTURE.md` — component map with file references.
- `docs/PROTOCOL.md` — every WebSocket message, in and out.

@.claude/wiz-claude.md
