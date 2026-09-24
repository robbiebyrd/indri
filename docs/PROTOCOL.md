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

### Pseudo-actions: `received` and `processed`

`router.Act` runs handlers registered under the literal action `received` before, and `processed` after,
the message's real action. Registering a handler on either gives you a pre/post hook that fires on every
inbound message. Nothing is registered on them by default.

---

## Server → client

### Transport

Server→client messages are **MessagePack binary** by default. Connect with `?debug=1` to receive JSON
text instead, which is useful for human-readable inspection. The WebSocket client must set
`binaryType = "arraybuffer"`.

### Layout frame

Sent once per new schema version, immediately before the keyframe, on `create`, `join`, and `reconnect`.
Not sent on `refresh`.

```json
{ "o": 4, "v": "<8-char hex hash>", "data": { "layout": { … } } }
```

- `o: 4` is the layout opcode.
- `v` is the layout schema version hash, stable across game instances using the same config.
- `data` contains the layout object from the script config.

### Slim keyframe wrapper

Follows the layout frame. `PrivateData` and the `layout` key are stripped from `data` before sending.

```json
{ "sv": "<same hash as v in layout frame>", "game": { … } }
```

- `sv` is the schema version (matches `v` from the preceding layout frame). Presence of `sv` identifies
  this message as a keyframe.
- `game` is a sanitized `models.Game`. The client must build its positional map from `game` before
  merging the cached layout back for rendering.

### Delta — change event

Published by the store at each write and broadcast to every session in the affected game.

```json
{ "o": 1, "t": "2024-01-15T12:00:00Z", "u": [[[2, 0, 1], "X"]], "r": [] }
```

Fields:
- `o` — opcode: `1` = update, `2` = insert, `3` = delete
- `t` — timestamp. In binary (MessagePack) mode: a Timestamp extension type, decoded to a `Date` by
  `@msgpack/msgpack`. In debug (JSON) mode: an RFC3339 string. Both are accepted by `new Date(t)`.
- `u` — array of `[path, value]` pairs; each `path` is a positional integer array
- `r` — array of paths to remove; each path is a positional integer array

In debug mode (`?debug=1`), paths are numeric-dotted strings instead of integer arrays: `"2.0.1"` is
equivalent to `[2, 0, 1]`.

**Positional path encoding:** integers index into the slim keyframe's key schema by sorted alphabetical
position at each object level; arrays use raw numeric indices. For example, if the slim keyframe root
has sorted keys `[code, data, players, stage, teams, updatedAt, …]` then `players` is at index `2`. If
a player object has sorted keys `[connected, controller, data, host, name, score, userId]` then `host`
is at index `3`. So the string path `players.p0.host` encodes as `[2, <p0 sorted position>, 3]`.

The client rebuilds the positional map from the slim keyframe at join/reconnect time using
`buildPositionalMap`, then passes the schema to `GameStateParser.setSchema` or lets `set()` auto-initialize it.

**Important invariant:** keys must never be deleted from the game state — only set to `null`. Deleting a
key shifts all subsequent positional indices and breaks decoding.

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

1. `"sv" in msg` → slim keyframe wrapper
2. `msg.o === 4` → layout frame
3. `msg.o === 1, 2, or 3` → delta update
4. `authenticated === true` → auth payload
5. `op === "inquiryResponse"` → game list
6. `"disconnected" in msg` → disconnect

Note: `sv` is checked before `authenticated` — keyframe priority is by design.
