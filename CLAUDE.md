# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

Indri is a Go backend for real-time, multiplayer, browser/mobile party games. Clients talk to it over a
single WebSocket endpoint (`/ws`) using JSON messages routed by an `action` field. Game state lives in
MongoDB; each write computes its own delta in application code and publishes it on an event bus, which
broadcasts it to everyone in that game. `client/` is a companion Expo/React Native reference client.

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

go run ./cmd/server -script ./config.json             # run (defaults to ./config.json)
go run ./example/tictactoe -script ./config.json      # run the tic-tac-toe example
air                                                    # hot reload (.air.toml)

# Infrastructure (MongoDB + Redis) — requires a populated .env
docker compose up -d

# Client (client/, pnpm only — there is no package-lock.json)
pnpm install
pnpm run typecheck
pnpm test          # node --experimental-strip-types on services/game-state-parser.node-test.ts
pnpm run lint
pnpm run web       # or: start / ios / android
```

Config is environment-driven with an `INDRI_` prefix (`internal/repo/env/env.go`); `.env.example`
documents every variable. Copy it to `.env` before running anything.

MongoDB does **not** need to be a replica set — deltas are computed by the application, not read from a
change stream. `docker-compose.yml` still starts one (`rs0`, initiated in its healthcheck), which is
harmless.

## Architecture

### Boot chain

`cmd/server/main.go` → `boot.Boot(ctx, scriptPath)` → `boot.Serve(i)`.

`Boot` wires a three-layer injector (`internal/injector/`), in strict order:

1. **clients** (`clients.go`) — MongoDB, the melody WebSocket hub, the lock manager, and the change-event
   publisher. Clients are process-global singletons; `GetClients` returns a cached `*ClientsInjector` on
   every call after the first, and accepts overrides for tests. `INDRI_LOCK_BACKEND=redis` is the
   *multi-instance switch*: it flips **both** the lock manager and the event bus to Redis, sharing one
   connection. Otherwise both are in-process.
2. **repos** (`repos.go`) — game/user/session Mongo stores plus the script store (loaded from the JSON
   file). Stores create their own indexes in `NewStore`.
3. **services** (`services.go`) — business logic over the repos.

`*injector.Injector` embeds all three, so handlers reach anything via one struct (`h.i.GameService`,
`h.i.GameRepo`, `h.i.MelodyClient`, …). Every handler takes it in `New(i *injector.Injector)`.

`Serve` runs two goroutines under an `errgroup`: the HTTP/WebSocket server and the change-stream
broadcaster. Both shut down on root-context cancellation (SIGINT/SIGTERM), then `closeResources` drains
the hub and the Mongo pool.

### Inbound: message routing

`melody.HandleMessage` → `router.HandleMessage` (`internal/handlers/router/`):

1. `utils.DecodeMessageWithAction` unmarshals to `map[string]interface{}`, pulls out `action`, and
   **deletes `action` from the payload** before handing it on.
2. `router.Act` runs three pseudo-phases per message: **`received` → `<action>` → `processed`**. Any
   handler registered under the literal actions `received` or `processed` therefore runs on *every*
   message — that is the intended pre/post hook mechanism. Nothing registers them by default.
3. `invokeHandler` wraps each call in a `recover()`, converting a panic into an error so a malformed
   client message can't kill the process or leak the melody session.

Handlers are a flat, ordered `[]Handler` registry (`register.go`) — multiple handlers may share one
action, and all matching handlers run. `boot.registerHandlers` installs the built-ins; a game adds its
own with `router.RegisterHandler(name, action, handler)` **after** `boot.Boot` and before `boot.Serve`
(see `example/tictactoe/main.go`).

### Outbound: keyframes and deltas

Two different shapes reach the client:

- **Keyframe** — a full, sanitized `models.Game` written directly to the requesting connection (on
  join/create/refresh/reconnect). `GameService.Sanitize` strips `PrivateData` from the game, stage,
  scenes, teams, and players before it goes out.
- **Delta** — an `events.ChangeEvent` (`{id, op, ts, type, updated, removed}`) broadcast to every session
  in the affected game.

Both paths are sanitized, and deliberately in step: `Store.publish` runs every delta through
`events.SanitizeDelta`, which drops any dotted path containing a `privateData` segment and recursively
strips `privateData` out of whole-object update values (e.g. a newly added player). If you change what
`GameService.Sanitize` hides, change `SanitizeDelta` to match or the two disagree.

The delta path is **application-computed, not database-driven** (`internal/services/events`; there is no
MongoDB change stream):

```
store write ──► events.Diff(before, after) ──► Publisher.Publish
                                                     │
                       in-process channel ───────────┤
                       or Redis Pub/Sub  ────────────┘
                                                     ▼
                              boot.monitorGameChanges (Subscribe)
                                                     ▼
                              BroadcastService.Broadcast(gameID, …)
```

`events.Diff` walks the **JSON** representation of before/after (`events.ToMap`), so paths look like
`players.<id>.host` and use json tag names, matching what the client already holds. Nested maps are
walked; arrays and scalars are compared whole with `reflect.DeepEqual`.

The client (`client/services/game-state-parser.ts`) rebuilds state by cloning the last keyframe and
replaying timestamp-ordered deltas over it, discarding deltas older than the keyframe's cutoff.

**A write that never publishes is invisible to players.** This is now the single most important rule in
the repo, because nothing in the database enforces it:

- `Store.Mutate` snapshots the game before `apply`, diffs it after a committed save, and publishes
  automatically. Prefer it.
- `UpdateField`, `DeleteField`, and `markPlayerConnected` publish explicitly via `s.publish(...)`. Any
  new write path must do the same.
- Publish failures are logged, never returned — the write already committed, so a fan-out hiccup must
  not fail the mutation.

`BroadcastService` offers game, team, player, and global fan-out. Because `sessionId` is the only
per-connection key the app sets, every targeted send resolves its recipients through the session store
first and then filters connections on that one key (`broadcastToSessions`). Do not invent new melody
session keys to filter on — nothing sets them.

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
- `gameRepo.Store.Mutate` is the concrete binding. **Use `Mutate` for any read-modify-write on a game.**
  Multi-field edits (add player to team, change team, set host) all go through it; see
  `internal/repo/game/{player,team,host}.go`.
- Single-field sets that don't depend on prior state can use `UpdateField`/`DeleteField`, which
  `$inc` the version so they stay coherent with `Mutate`'s CAS.

Nothing here is Mongo-specific by design: a different backend only needs to supply `load` and a
version-conditional `save`.

### Sessions and identity — the `sessionId` overload

There are **two different values both called `sessionId`**, and confusing them is a security bug:

| Where | Value | Notes |
|---|---|---|
| Melody connection key `sessionId` | `session.ID.Hex()` (Mongo ObjectID) | Server-side targeting key for broadcasts. Never sent to clients. |
| JSON field `sessionId` on the wire | `session.Token` (256-bit hex) | Unguessable bearer token. Client echoes it back in a `reconnect`. |

`login` and `reconnect` are the only places that call `SetKey`, and the only key they set is `sessionId`.
Authorization must always resolve the caller from *their own connection's* `sessionId`, never from a
client-supplied `userId` — `kick` is the reference implementation.

Sessions are one-per-user: `sessionRepo.New` returns the existing session for a `userId` rather than
creating a second one (unique index on `userId`).

### Data model

MongoDB collections: `game`, `user`, `session`. Models live in `internal/models/`.

A **Game** holds `Teams`, `Players`, and a `Stage`. A **Stage** holds ordered **Scenes** plus a
`currentScene`; a scene is the visual unit the client renders. Games, stages, scenes, teams, and players
each carry up to three data stores:

- `PublicData` (`data`) — broadcast to everyone in the game.
- `PrivateData` (`privateData`) — server-only; stripped by `Sanitize`.
- `PlayerData` (`playerData`) — keyed per player.

A **Script** (`config.json`, `models.Script`) is the template a new game is stamped from: config, initial
teams, and the initial stage/scenes. It is loaded once at boot from `-script` and exposed as
`i.Script`.

## Adding a game action

1. Create `example/<game>/server/handlers/<action>/handler.go` with a `Handler` struct holding
   `*injector.Injector`, a `New(i)` constructor, and
   `Handle(s *melody.Session, decodedMsg map[string]interface{}) error`.
2. Resolve the caller: `connection.NewService(s, h.i.MelodyClient).GetKeyAsString("sessionId")` →
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

## Known rough edges

Do not treat these as intentional; check before relying on them.

- `docs/tasks.md` is an aspirational backlog, not a description of current state. Several of its
  entries (dependency injection, context propagation, CI) are already done.

## Reference docs

- `docs/ARCHITECTURE.md` — component map with file references.
- `docs/PROTOCOL.md` — every WebSocket message, in and out.

@.claude/wiz-claude.md
