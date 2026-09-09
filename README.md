# Indri

A Go framework for real-time multiplayer party games.

Indri gives you the parts every "everyone grab your phone" game needs — accounts, sessions, lobbies,
rooms, teams, hosts, reconnection, and live state sync — so a game is reduced to a JSON template and a
handful of action handlers. Players connect over a single WebSocket; state lives in MongoDB and every
write is diffed and pushed back to the room as a delta.

Released into the public domain ([Unlicense](LICENSE)).

## How it works

```
client ──ws──► action router ──► handler ──► store ──► MongoDB
                                               │ diff
client ◄──────── broadcast ◄─── event bus ◄────┘
```

Clients send `{"action": "...", ...}`. The router dispatches on `action`. Handlers mutate the game
document; the store diffs the before/after state and publishes the delta; the server fans it out to
everyone in that game. A client holds the last full snapshot (a *keyframe*) and replays deltas over it.

Because the server is the sole writer it computes deltas itself rather than tailing a database change
feed — so MongoDB needs no replica set, and the same mechanism would work over any store. On a single
instance the bus is an in-memory channel; set `INDRI_LOCK_BACKEND=redis` and it becomes Redis Pub/Sub,
so deltas reach players connected to other instances.

Everything a specific game needs to define is:

1. **A script** — a JSON file describing the starting teams, stage, and scenes (`config.json`).
2. **Action handlers** — one Go package per verb your game understands.

`example/tictactoe/` is a complete game in two files: a script, one `move` handler, and one
`router.RegisterHandler` call in `main.go`.

## Quick start

Requires Go 1.25+, Docker (for MongoDB and Redis), and pnpm if you want the reference client.

```bash
cp .env.example .env       # then edit as needed
docker compose up -d       # MongoDB + Redis
go run ./cmd/server -script ./config.json
```

The server listens on `INDRI_LISTEN_ADDRESS:INDRI_LISTEN_PORT` (default `localhost:5002`) and exposes
one endpoint, `/ws`.

The bundled `docker-compose.yml` starts MongoDB as a single-node replica set (`rs0`) and Redis. The
replica set is not required — it is a leftover from when deltas came from a change stream — but it is
harmless.

To run the example game instead:

```bash
go run ./example/tictactoe -script ./config.json
```

### Reference client

```bash
cd client
pnpm install
pnpm run web        # or: ios / android
```

Point it at the server with `EXPO_PUBLIC_API_URL=ws://localhost:5002/ws`.

## Talking to the server

Register, log in, then create or join a room:

```json
{ "action": "register", "email": "user@example.com", "password": "SuperSecret",
  "name": "A User", "displayName": "!!u532!!" }

{ "action": "login", "email": "user@example.com", "password": "SuperSecret" }
→ { "authenticated": true, "sessionId": "<token>", "user": { "id": "…", "score": 0, … } }

{ "action": "create", "code": "my-room", "teamId": "team1" }
{ "action": "join",   "code": "my-room", "teamId": "team2" }
→ full game keyframe
```

The `sessionId` you get back is a secret bearer token. Keep it, and send
`{"action": "reconnect", "sessionId": "<token>"}` to resume a dropped connection — the server restores
the player's game and team and re-sends the current state.

Built-in actions: `register`, `login`, `logout`, `reconnect`, `create`, `join`, `leave`, `kick`,
`refresh`, `inquire`. Full request and response shapes are in **[docs/PROTOCOL.md](docs/PROTOCOL.md)**.

## Data model

A **Game** has **Players**, **Teams**, and a **Stage**. A Stage holds ordered **Scenes** and tracks
which one is current; a Scene is the unit of screen your client renders.

Games, stages, scenes, teams, and players each carry three data stores:

| Store | Key | Who sees it |
|---|---|---|
| Public | `data` | everyone in the game |
| Private | `privateData` | the server only — stripped before every send |
| Player | `playerData` | keyed per player |

That split is what lets you keep a hidden answer, a shuffled deck, or a secret role on the same document
as the board everyone is looking at.

## Configuration

Everything is environment-driven with an `INDRI_` prefix, and `.env.example` documents each variable.
The ones you are most likely to change:

| Variable | Default | Purpose |
|---|---|---|
| `INDRI_LISTEN_ADDRESS`, `INDRI_LISTEN_PORT` | `localhost`, `5002` | Where to listen |
| `INDRI_ALLOWED_ORIGINS` | *(empty)* | Comma-separated browser origins allowed to open a socket. Empty rejects all cross-origin browsers; clients that send no `Origin` are always allowed |
| `INDRI_MONGO_URI`, `INDRI_MONGO_DATABASE` | `localhost`, `indri` | Database |
| `INDRI_LOCK_BACKEND` | `inprocess` | Multi-instance switch. `redis` moves both the edit locks and the delta bus to Redis |
| `INDRI_WS_*` | see `.env.example` | Timeouts, ping interval, message size limits |

## Concurrency

Several players routinely edit the same game at the same moment. Indri serializes those edits with a
distributed lock and commits behind a version fence, so a lock lease expiring mid-write still cannot
produce a lost update. The coordination lives in `internal/services/mutation`, above the database — a
store only has to know how to load a document and how to save it conditionally on its version.

Single-instance deployments use in-process locks and an in-process delta bus. Set
`INDRI_LOCK_BACKEND=redis` to move both to Redis and run more than one instance.

## Project layout

```
cmd/server/          entry point
internal/
  clients/           MongoDB, Redis, melody WebSocket hub
  entrypoints/       HTTP server, WebSocket lifecycle
  handlers/          action router + one package per built-in action
  services/          game, stage, broadcast, auth, session, mutation, lock, events
  repo/              MongoDB stores, environment config, script loader
  models/            Game, Stage, Scene, Team, Player, User, Session, Script
  injector/          three-stage dependency wiring
example/tictactoe/   worked example game
client/              Expo / React Native reference client
docs/                architecture and protocol reference
```

## Development

```bash
go build ./...
go vet ./...
go test -race ./...      # what CI runs
air                      # hot reload

cd client && pnpm run typecheck && pnpm test
```

Tests that need MongoDB skip themselves when none is reachable, so `go test ./...` is safe on a bare
checkout. Set `INDRI_TEST_MONGO_URI` to point them at a database.

## Further reading

- **[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)** — component map, request lifecycle, concurrency model
- **[docs/PROTOCOL.md](docs/PROTOCOL.md)** — every WebSocket message, in and out
- **[CLAUDE.md](CLAUDE.md)** — orientation for AI coding agents
- **[docs/tasks.md](docs/tasks.md)** — improvement backlog
