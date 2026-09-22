# Lua Lifecycle Hooks Design

**Date:** 2026-09-22
**Project:** Indri

## Overview

Game scripts defined in `config.json` need to react to framework lifecycle events (join, leave, kick, inquire) and trigger full game-state refreshes from within handler scripts. This spec covers the new Lua stdlib surface and the minimal Go changes required to support it.

## Scope

- Lifecycle hooks for `join`, `leave`, `kick`: run as full game-mutation Lua handlers after the framework action completes.
- Inquiry notification hook for `inquire`: run as a lightweight notification-only Lua handler after the framework action completes.
- Two new Lua stdlib functions: `indri.refresh()` and `indri.refreshSelf()`.
- `caller` table enriched with `userId`.

Out of scope: replacing or overriding framework handler behavior; customising the inquire response payload; pre-action hooks.

## Wiring and Execution Model

Registration order in `boot.Boot` already guarantees framework-first: `registerHandlers` runs before `luahandler.Register`, so the Go handler always completes before any Lua hook fires.

`luahandler.Register` iterates `i.Script.Handlers` and registers each action. The only change is a fork at registration time:

- Action `"inquire"` → `InquireNotificationHandler`
- All other actions (including `join`, `leave`, `kick`, and custom game actions like `move`) → existing `Handler`

### Session state at hook execution time

| Action | Session state when Lua fires |
|---|---|
| `join` | Framework handler has updated session with `gameID`/`teamID` — `gameAndTeamFromSession` works normally |
| `leave` | Framework handler has removed player from game but does not clear session `gameID` — `gameAndTeamFromSession` still returns valid IDs |
| `kick` | `caller` = kicker (from their own session); `msg.userId` = kicked player's user ID |
| `inquire` | No game context required — `InquireNotificationHandler` does not look up a session |

## New Lua Stdlib Surface

### `caller` enrichment

`gameAndTeamFromSession` is extended to return `userId` in addition to `gameId` and `teamId`. The `caller` table becomes:

```lua
caller.teamId  -- unchanged
caller.userId  -- new: the acting player's user ID
```

This is additive and does not break existing scripts.

### `indri` global table

A new `indri` table is registered as a Lua global before every in-game handler script runs:

```lua
indri.refresh()      -- broadcast full sanitized game state to all players in the game
indri.refreshSelf()  -- send full sanitized game state to the triggering connection only
```

Both functions are implemented as Go functions registered via `L.SetField`. Calling either sets a flag on a `postMutateActions` struct (`struct { refreshAll, refreshSelf bool }`) held by the handler for the duration of the `Handle` call.

The actual sends happen **after** `GameRepo.Mutate` returns (once the write is committed). Inside the apply callback, calling `indri.refresh()` only records intent — it cannot fire early against an uncommitted state.

Post-Mutate execution:
- `refreshAll`: `GameService.GetJSONBytes(gameId)` → broadcast to all sessions in the game
- `refreshSelf`: `GameService.GetJSONBytes(gameId)` → `connection.Service.Write` on the triggering `transport.Conn`

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
| `internal/handlers/luahandler/handler.go` | Add `postMutateActions` struct; extend `gameAndTeamFromSession` to return `userId`; register `indri` table before script runs; execute post-Mutate broadcasts after `Mutate` returns |
| `internal/handlers/luahandler/register.go` | Fork on `"inquire"` to register `InquireNotificationHandler`; all others use `Handler` |
| `internal/handlers/luahandler/inquire_handler.go` | New — `InquireNotificationHandler` with `Handle` (msg-only, no Mutate) |
| `internal/handlers/luahandler/runtime_test.go` | Add: `caller` table includes `userId` |
| `internal/handlers/luahandler/handler_test.go` | New — tests for `indri.refresh()`/`indri.refreshSelf()` flag setting, post-Mutate broadcast execution, `InquireNotificationHandler.Handle` |

No changes to `stdlib.go`, `runtime.go`, or any handler outside `luahandler/`.

## Error Handling

- `gameAndTeamFromSession` returning an error on a lifecycle hook (e.g. session not found after a rapid disconnect) is handled the same as today: the error is returned and logged by the router's `invokeHandler` recover wrapper.
- If `GameService.GetJSONBytes` fails during a post-Mutate refresh, log and continue — the mutation already committed successfully.
- `InquireNotificationHandler` script errors are returned as errors (same recover wrapper applies).

## Testing

- **`indri.refresh()` / `indri.refreshSelf()`**: inject mock broadcast/write into handler, run a script calling each function, assert the correct send fired after `Mutate`.
- **`InquireNotificationHandler`**: construct directly, call `Handle` with a fake `transport.Conn`, assert `msg` is accessible from Lua and the script runs without error.
- **`caller.userId`**: assert the field is present and correct in the `caller` table.
- Existing tests are unaffected — all changes are additive.
