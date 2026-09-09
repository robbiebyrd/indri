# AGENTS.md

Orientation for AI coding agents working in this repository.

The full guidance lives in **[CLAUDE.md](CLAUDE.md)** — read it first. It covers build/test commands,
the boot and dependency-injection chain, the message-routing lifecycle, the concurrency model, how to
add a game action, and a list of known defects you should not mistake for intentional design.

Supporting references:

- **[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)** — component map with file paths.
- **[docs/PROTOCOL.md](docs/PROTOCOL.md)** — the complete WebSocket message contract.

## The short version

Indri is a Go WebSocket backend for real-time multiplayer games, plus an Expo/React Native reference
client in `client/`.

```bash
go build ./... && go vet ./... && go test -race ./...   # server
cd client && pnpm install && pnpm run typecheck && pnpm test
```

Four things trip up newcomers:

1. **`sessionId` means two different things.** On the wire it is a secret bearer token; on the melody
   connection it is the session's Mongo ObjectID. Never send the latter or trust the former from
   anywhere but the caller's own connection.
2. **A write that doesn't publish a change event is invisible to players.** Deltas are computed in
   application code (`internal/services/events`), not by a database change stream. `Store.Mutate`
   publishes automatically; every other write path must call `s.publish(...)` itself.
3. **Read-modify-writes on a game must go through `gameRepo.Store.Mutate`**, which serializes under a
   lock and commits behind a version fence.
4. **Each `repo/*` package asserts `var _ Storer = (*Store)(nil)`** — add an exported `Store` method
   and you must add it to `interface.go` too, or the build breaks.
