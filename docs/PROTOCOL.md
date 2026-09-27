# Indri WebSocket protocol

All traffic flows over a single endpoint: `ws://<host>:<port>/ws` (default `localhost:5002`).

Every message is a JSON object. Client→server messages **must** carry a string `action` field; the
router uses it to pick handlers and removes it from the payload before the handler sees it.

The upgrade is origin-checked (`internal/clients/melody/client.go`). Requests with no `Origin` header
(native apps, CLI tools, server-to-server) are always allowed; browser requests are allowed only if
their origin appears in `INDRI_ALLOWED_ORIGINS`. An empty allowlist rejects all cross-origin browsers.

---

## Handshake

On connect the server immediately sends:

```json
{ "stage": { "currentScene": "login" } }
```

## The two `sessionId` values

The JSON field `sessionId` carries a **secret 256-bit bearer token**, not the database id. The server
separately keys the connection by the session's Mongo ObjectID, which is never sent to clients. Store
the token and echo it back on `reconnect`; treat it like a password.

---

## Client → server

### `register`

```json
{ "action": "register", "email": "user@example.com", "password": "SuperSecret",
  "name": "A User", "displayName": "!!u532!!" }
```

Replies `{"registered": true, "userId": "<hex>"}`. `email` and `password` are required; the password is
bcrypt-hashed before storage. On a connection that has already authenticated, replies
`{"registered": false, "error": "user already logged in"}` instead.

### `login`

```json
{ "action": "login", "email": "user@example.com", "password": "SuperSecret" }
```

Replies with the authenticated payload:

```json
{
  "authenticated": true,
  "sessionId": "<64-char hex token>",
  "user": { "id": "…", "createdAt": "…", "updatedAt": "…", "email": "…",
            "name": "…", "displayName": "…", "score": 0 }
}
```

A user has at most one session; logging in again returns the existing one.

### `reconnect`

```json
{ "action": "reconnect", "sessionId": "<token from login>" }
```

Re-binds the stored session to this connection and replies with the same authenticated payload. If the
session was already in a game, a full game keyframe follows immediately.

### `create`

```json
{ "action": "create", "code": "my-room", "teamId": "team1", "private": false }
```

`code` and `teamId` are both required. Fails if a game with that code already exists. Replies with a
game keyframe. `private: true` hides the game from `availableGames`.

Note: `GameService.New` can auto-generate a three-word code when passed an empty one, but the `create`
handler currently requires a non-empty `code`.

### `join`

```json
{ "action": "join", "code": "my-room", "teamId": "team1" }
```

Adds the caller to the game and team (moving them if they were already on another team), replies with a
game keyframe, and records `gameId`/`teamId` on the session.

### `inquire`

```json
{ "action": "inquire", "inquiryType": "game", "inquiry": "availableGames" }
{ "action": "inquire", "inquiryType": "game", "inquiry": "gameInfo", "code": "my-room" }
```

Replies:

```json
{ "op": "inquiryResponse",
  "games": [ { "code": "my-room", "full": false,
               "teams": [ { "name": "Player 1", "full": true },
                          { "name": "Player 2", "full": false } ] } ] }
```

`availableGames` returns up to 100 non-private games.

### `refresh`

```json
{ "action": "refresh" }
```

Re-sends a full sanitized keyframe for the session's current game, or a `WSError` if there is none.

### `leave`

```json
{ "action": "leave" }
```

Removes the caller from their game and from any team, atomically.

### `kick`

```json
{ "action": "kick", "code": "my-room", "userId": "<target user id>" }
```

Host-only. The caller is authorized from their own connection's session — never from the payload. The
target is removed from the game and force-disconnected if currently connected.

### `logout`

```json
{ "action": "logout" }
```

Runs the same path as a disconnect: marks the player disconnected in their game, sends
`{"disconnected": true}`, and closes the connection. No-ops if the connection was never authenticated.

### `layout`

Host-only. Mutates `game.PublicData["layout"]` (the `data.layout` path on the wire) via a single
sub-operation. The caller is always authorized from their own connection's session key — never from
any `userId` in the payload.

Every layout message carries `action: "layout"`, the game `code`, and an `op` field that selects the
sub-operation. Unknown top-level keys are rejected.

#### Authorization

| Condition | Result |
|---|---|
| Connection has no `sessionId` key | reject — "must be logged in to edit a layout" |
| Session resolves but `UserID` is nil | reject — "calling session has no user id" |
| `session.GameID` ≠ target game | reject — "caller is not in game" |
| `g.Players[userId].Host` is false | reject — "caller is not the host" |
| Host, op fails structural validation | reject with the specific issue; no write |
| Host, valid op | `Mutate` → `Diff` → publish → broadcast |

#### Op table

| `op` | Additional required fields | Effect |
|---|---|---|
| `addWidget` | `sceneId`, `widgetId`, `widget` | Inserts or replaces a widget in the scene |
| `removeWidget` | `sceneId`, `widgetId` | Deletes the widget; no-op (ErrAbort) if absent |
| `setPlacement` | `sceneId`, `widgetId`, `placement` | Replaces the widget's placement (move or resize) |
| `setWidgetConfig` | `sceneId`, `widgetId`, `config` | Merges config keys into the widget's config |
| `setStyle` | `scope`, `style` (+ `sceneId` and/or `widgetId` per scope) | Sets style on board, scene, or widget |
| `setGrid` | `grid` | Replaces the root grid `{cols, rows}` |
| `setScript` | `scope`, `source` (+ `sceneId` and/or `widgetId` per scope) | Sets script source on board, scene, or widget |

`setPlacement` covers both move and resize (same field, different values).

#### Structural constraints enforced by the server

- Grid `cols` and `rows` must be integers in `[8, 4096]`.
- Placement `w` and `h` must be positive integers; `col` and `row` must be non-negative.
- Grid-kind placements must not overlap with other grid-kind placements in the same scene.
- Grid-kind placements must not exceed the grid bounds.
- Absolute-kind placement values must be percentage strings (`"10%"`), not pixel numbers.
- Sub-grid nesting depth is capped at 4.
- Total widget count (across all scenes, including sub-grid children) is capped at 300.
- Serialized layout size is capped at 256 KiB.
- The key `privateData` is reserved and rejected anywhere in the layout document.

#### Wire examples

```json
{ "action": "layout", "code": "MYGAME", "op": "addWidget",
  "sceneId": "scene1", "widgetId": "title",
  "widget": {"type": "text", "placement": {"kind": "grid", "col": 0, "row": 0, "w": 4, "h": 2},
              "config": {"text": "Hello"}} }

{ "action": "layout", "code": "MYGAME", "op": "setPlacement",
  "sceneId": "scene1", "widgetId": "title",
  "placement": {"kind": "grid", "col": 2, "row": 1, "w": 4, "h": 2} }

{ "action": "layout", "code": "MYGAME", "op": "setGrid",
  "grid": {"cols": 12, "rows": 8} }

{ "action": "layout", "code": "MYGAME", "op": "setStyle",
  "scope": "widget", "sceneId": "scene1", "widgetId": "title",
  "style": {"backgroundColor": "#003366"} }
```

#### Security note

The server validates structure but does not sandbox widget config contents. Host-supplied script
source (`setScript`) and image URIs (`addWidget` with `type: "image"`) are stored and served
verbatim. Deployers are responsible for URI allow-listing and any XSS mitigations appropriate to
their environment.

### Pseudo-actions: `received` and `processed`

`router.Act` runs handlers registered under the literal action `received` before, and `processed` after,
the message's real action. Registering a handler on either gives you a pre/post hook that fires on every
inbound message. Nothing is registered on them by default.

---

## Server → client

### Keyframe — full game state

A bare `models.Game` object; recognizable because it has both `id` and `code`. `PrivateData` is stripped
from the stage, every scene, every team, and every player before sending.

```json
{
  "id": "…", "code": "my-room", "createdAt": "…", "updatedAt": "…",
  "players": { "<userId>": { "name": "…", "score": 0, "connected": true,
                             "host": true, "controller": false, "data": {} } },
  "teams":   { "team1": { "name": "Player 1", "playerIds": ["<userId>"], "data": {"marker": "X"} } },
  "stage":   { "currentScene": "board", "sceneOrder": ["board"],
               "scenes": { "board": { "data": { "board": [["","",""],["","",""],["","",""]] } } } },
  "data": {}
}
```

Sent on `create`, `join`, `refresh`, and after a `reconnect` into an active game.

### Delta — change event

Published by the store at each write and broadcast to every session in the affected game.

```json
{ "id": "<game object id>", "op": "update", "ts": "2026-09-09T12:00:00Z", "type": "game",
  "updated": { "stage.scenes.board.data.board": [["X","",""],["","",""],["","",""]] },
  "removed": ["stage.scenes.board.data.temp"] }
```

Keys in `updated` are dotted paths into the game's **JSON** representation — the same field names the
keyframe uses. Nested objects are walked (`players.<userId>.host`); arrays and scalars are replaced
whole. Apply them onto the last keyframe.

Deltas are sanitized on the same terms as keyframes: paths containing a `privateData` segment are
dropped, and `privateData` is stripped out of whole-object update values. A client never sees private
data on either path.

### Error

```json
{ "errorCode": 3003, "message": "user must join a game first", "op": "error" }
```

| Code | Meaning |
|---|---|
| 1003 | user is already logged in |
| 1011 | internal server error |
| 3000 | authentication required |
| 3003 | no game / game not found / session not found / wrong team / wrong game |

Unauthenticated calls to `create`, `join`, and `inquire` instead receive
`{"authenticated": false, "stage": {"currentScene": "login"}}`.

### Disconnect

```json
{ "disconnected": true }
```

Sent before the server closes a connection it still owns (for example, a kick).

---

## Client-side message classification

The reference client (`client/services/message-handler.ts`) routes by shape, in this order:

1. `authenticated === true` → auth payload
2. `op === "update"` → delta
3. `op === "inquiryResponse"` → game list
4. has both `code` and `id` → keyframe
5. any other `op` → dispatched by that name (this is how game-specific messages get through)
