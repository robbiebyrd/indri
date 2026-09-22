# Lua Lifecycle Hooks Design

**Date:** 2026-09-22
**Project:** Indri

## Overview

Game scripts defined in `config.json` need to react to framework lifecycle events (join, leave, kick, inquire) and trigger full game-state refreshes from within handler scripts. This spec covers the new Lua stdlib surface and the minimal Go changes required to support it.

## Scope

- Lifecycle hooks for `join`, `leave`, `kick`: run as full game-mutation Lua handlers after the framework action completes.
- Inquiry notification hook for `inquire`: run as a lightweight notification-only Lua handler after the framework action completes.
- Two new Lua stdlib functions: `indri.refresh()` and `indri.refreshSelf()`.
- `caller` table enriched with `userId` and `gameId`.

Out of scope: replacing or overriding framework handler behavior; customising the inquire response payload; pre-action hooks.

## Wiring and Execution Model

Registration order in `boot.Boot` already guarantees framework-first: `registerHandlers` runs before `luahandler.Register`, so the Go handler always completes before any Lua hook fires.

`luahandler.Register` iterates `i.Script.Handlers` and registers each action. The only change is a fork at registration time:

- Action `"inquire"` → `InquireNotificationHandler`
- All other actions (including `join`, `leave`, `kick`, and custom game actions like `move`) → existing `Handler`

### Session state at hook execution time

| Action | Session state when Lua fires | Game state when Lua fires |
|---|---|---|
| `join` | Session `gameID`/`teamID` are set on the success path — `SessionService.Update` is the last statement of the Go join handler and has committed before the Lua hook fires. Any failure after `ConnectPlayer` (including `GetJSONBytes` or `Write`) causes the handler to return early, which propagates up and prevents the Lua hook from running at all. | Player is **likely** present in `g.Players`, but not guaranteed: `ConnectPlayer` errors are logged and swallowed, so the join handler may reach `SessionService.Update` even if the player was not added to the game. Scripts must guard against a missing `g.Players[caller.userId]`. |
| `leave` | Session `gameID` is NOT cleared by the leave handler (a known gap — see CLAUDE.md "Known rough edges"). `gameAndTeamFromSession` returns valid IDs because the session record still holds stale values set at join time. This is load-bearing: if session cleanup is ever added to the leave handler, the Lua hook will silently lose its game context. `g.Players[caller.userId]` and team membership have already been removed by `GameService.RemovePlayer`. Scripts must not assume the leaving player still exists in game state. | Player is absent from `g.Players` and all team `PlayerIDs`. |
| `kick` | `caller` = kicker (from their own session); `msg.userId` = kicked player's user ID. The kicked player has already been removed from the game and their connection force-closed by `HandleDisconnect` before the Lua hook fires. Scripts must not assume the kicked player still exists in game state. | Kicked player is absent from `g.Players`; their connection is closed. |
| `inquire` | No game context required — `InquireNotificationHandler` does not look up a session. The Go inquire handler validates session presence before returning, so the hook fires only for authenticated connections — but the hook does not expose session or caller identity. This is intentional: the notification is game-level, not player-level. | No game context. |

## New Lua Stdlib Surface

### `caller` enrichment

`gameAndTeamFromSession` is refactored to call `SessionService.Get` directly (replacing the current `SessionService.GetGameIDAndTeamID` call) and read `session.GameID`, `session.TeamID`, and `session.UserID` from the returned `Session` struct in a single round trip. (`GetGameIDAndTeamID` was never part of the `SessionService` interface, so no interface file changes are needed either way; the real benefit is reading all three fields without a second database call.) The `caller` table becomes:

```lua
caller.teamId  -- unchanged
caller.userId  -- new: the acting player's user ID
caller.gameId  -- new: the game the action was performed in
```

No changes to `internal/services/session/` are required. This is additive and does not break existing scripts.

### `indri` global table

A new `indri` table is registered as a Lua global before every in-game handler script runs:

```lua
indri.refresh()      -- broadcast full sanitized game state to all players in the game
indri.refreshSelf()  -- send full sanitized game state to the triggering connection only
```

Both functions are implemented as Go functions registered via `L.SetField`. Calling either sets a flag on a `postMutateActions` struct (`struct { refreshAll, refreshSelf bool }`) allocated in `Handle` **before** the `Mutate` call and captured by the `indri` table closures. It is intentionally allocated outside the `Mutate` closure so it survives retries: `Mutate` may retry up to 10 times, re-executing the apply callback with a fresh `lua.LState` each time, but all retry attempts capture the same `postMutateActions` pointer. Boolean flags set by any retry attempt persist across subsequent attempts. This is safe because Lua game-handler scripts are expected to be deterministic — a script that calls `indri.refresh()` will do so on every retry, and one that does not will not. A non-deterministic script that conditionally calls `indri.refresh()` on some retries but not others would see unexpected behaviour (a refresh firing despite the final retry not requesting one); this is an accepted trade-off and script authors should keep handlers free of side-effect-conditional branches.

The actual sends happen **after** `GameRepo.Mutate` returns successfully (once the write is committed) — exactly once, regardless of how many retry attempts occurred. `GetJSONBytes`/`Sanitize` is called once at that point to fetch the committed state; it is never called inside the retry loop. Inside the apply callback, calling `indri.refresh()` only records intent — it cannot fire early against an uncommitted state.

`s transport.Conn` is captured at `Handle` entry and held alongside `postMutateActions` for use by `indri.refreshSelf()`. If the triggering connection has closed between `Mutate` returning and the post-Mutate send, the write error is logged and discarded — consistent with the existing publish-failure policy: the mutation already committed.

Post-Mutate execution:
- `refreshAll`: `GameService.Get(gameId)` → `GameService.Sanitize(g)` → `BroadcastService.Broadcast(&gameId, nil, sanitizedGame)` (passes `nil` teamId for whole-game fan-out; `Broadcast` marshals the struct internally — raw `[]byte` must not be passed)
- `refreshSelf`: `GameService.Get(gameId)` → `GameService.Sanitize(g)` → marshal to JSON → `connection.Service.Write` on the captured `transport.Conn`

The `indri` table is **not** available in `InquireNotificationHandler` (there is no game to refresh against).

## InquireNotificationHandler

A new, thin handler type for the `inquire` lifecycle hook:

- Creates a fresh Lua state and loads stdlib.
- Sets only `msg` as a global (the decoded message payload).
- Runs the script.
- No session lookup, no `Mutate`, no `game`/`caller`/`initial`/`indri` globals.
- Return value and mutations are ignored — this is purely a notification.

## Files

| File | Change |
|---|---|
| `internal/handlers/luahandler/handler.go` | Add `postMutateActions` struct; extend `gameAndTeamFromSession` to return `gameId` and `userId`; populate `caller.gameId` and `caller.userId`; register `indri` table (capturing `s transport.Conn`) before script runs; execute post-Mutate broadcasts after `Mutate` returns |
| `internal/handlers/luahandler/register.go` | Fork on `"inquire"` to register `InquireNotificationHandler`; all others use `Handler` |
| `internal/handlers/luahandler/inquire_handler.go` | New — `InquireNotificationHandler` with `Handle` (msg-only, no Mutate) |
| `internal/handlers/luahandler/handler_test.go` | New — tests for `indri.refresh()`/`indri.refreshSelf()` flag setting; post-Mutate broadcast execution; `caller.gameId`/`caller.userId` fields present; `InquireNotificationHandler.Handle` |

No changes to `stdlib.go`, `runtime.go`, `runtime_test.go`, or any handler outside `luahandler/`.

## Error Handling

- `gameAndTeamFromSession` returning an error on a lifecycle hook (e.g. session not found after a rapid disconnect) is handled the same as today: the error is returned and logged by the router's `invokeHandler` recover wrapper.
- If `GameService.GetJSONBytes` fails during a post-Mutate refresh, log and continue — the mutation already committed successfully.
- If the triggering connection is closed before `refreshSelf` executes, the write error is logged and discarded.
- `InquireNotificationHandler` script errors are returned as errors (same recover wrapper applies).

## Testing

All new tests go in `internal/handlers/luahandler/handler_test.go` (new file).

- **`indri.refresh()` / `indri.refreshSelf()`**: inject mock broadcast/write into handler, run a script calling each function, assert the correct send fired after `Mutate`.
- **`caller` enrichment**: assert `caller.gameId` and `caller.userId` are present and correct.
- **`InquireNotificationHandler`**: construct directly, call `Handle` with a fake `transport.Conn`, assert `msg` is accessible from Lua and the script runs without error.
- Existing tests in `runtime_test.go` and `stdlib_test.go` are unaffected — all changes are additive.
