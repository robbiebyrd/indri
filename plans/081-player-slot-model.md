# Story 081 — player slot model

Reference: local `main` (origin/main + `fix/game-membership`), not origin/main `0f85d3b`. Scope agreed
with Boss: slot model + 92850e4 + 9f18199 + origin's inquire fix 6db999e. The session-lifecycle half of
`fix/game-membership` is story 089.

## Model

- `Player.UserID` (`userId`) — the slot's holder; `""` means the slot is free.
- `Session.SlotID` (`slotId`) on `Session`, `CreateSession`, `UpdateSession`.
- `newGameDoc` pre-declares slots from the script: teams sorted by name, team *i* owns `p{i*max}…`,
  `Teams[t].PlayerIDs` lists them. The script's own teams map is copied, never written.

## Storer (repo/game)

Removed: `HasPlayerOnTeam`, `ChangePlayerTeam`, `AddPlayerToTeam`, `RemovePlayerFromTeam`,
`PlayerOnWhichTeam`, `HasPlayer`, `PlayerOnATeam`, `AddPlayer`, `AddPlayerResult`, `RemovePlayerResult`,
and the `docs.setPlayerConnected` port method on all three backends.

Added / changed, all on `core` over `Mutate` (so every backend gets them):

- `AssignSlot(id, teamId, userId, displayName) (slotId string, seated bool, err error)` — same team →
  keep and reconnect slot; other team → move to its first free slot, host flag included; full team →
  error and the current slot is kept. `seated` is true only when the user held no slot before: it is
  what `player:joined` fires on (replaces `AddPlayerResult`).
- `RemovePlayer(id, slotId, userId) error` — frees the slot (key, score and data kept until another
  user is seated there, who starts clean — Boss's call after review); errors if `userId`
  does not hold it, so of two racing removals exactly one returns nil (replaces `RemovePlayerResult`).
- `ConnectPlayer` / `DisconnectPlayer(id, slotId, userId)` — error on a missing slot, silent no-op
  (`ErrAbort`) when `userId` no longer holds it.

## Callers

- `GameService.ConnectPlayer(id, teamId, userId, name) (slotId, error)`; requires a team.
  `DisconnectPlayer`/`RemovePlayer` take `slotId, userId`. Lifecycle payloads gain `slotId`.
- join/create store `SlotID` on the session. leave, logout and the WS close path use `session.SlotID`.
- `handlerUtils.RequireHost(session, game)` — pure check shared by kick and layout: caller in this
  game, holds the slot their session names (92850e4), and that slot is host.
- kick takes `slotId` on every transport (WS, GraphQL arg, REST route, PROTOCOL.md). It frees the
  slot even when the target's session is elsewhere or missing, and only disconnects a target whose
  session is in this game (9f18199). A failed removal still disconnects nobody.
- inquire: a team is full only when every slot is held (6db999e).
- Lua `checkPlayers` also refuses a `userId` change — the key set is fixed now, so the key check alone
  no longer sees a membership change.
- client: `Player.userId`; `isHost`, `getTeam` and `LoggedIn` find the caller's slot by `userId`.
