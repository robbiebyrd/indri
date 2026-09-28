# Indri protocol

The messages below are the same on every transport; only the framing differs (see
[Transports](#transports)). By default the server speaks WebSocket at `ws://<host>:<port>/ws`
(default `localhost:5002`); `INDRI_TRANSPORTS` can enable others alongside or instead of it.

Every message is a JSON object. Client→server messages **must** carry a string `action` field; the
router uses it to pick handlers and removes it from the payload before the handler sees it.

Every transport is origin-checked (`internal/transport/origin.go`). With `INDRI_ALLOWED_ORIGINS`
empty (the default), every origin is accepted. That is safe because connections carry no cookies or
other ambient credentials: a client authenticates with `login` or a token it holds, so a foreign page
that opens a connection can't act as a player. With a list set, only its origins are accepted, plus
requests with no `Origin` header (CLI tools, server-to-server, most native clients). React Native's
iOS WebSocket sends the server's own origin, so list that origin when you set a list.
The HTTP-based endpoints also answer CORS preflights for allowed origins.

---

## Transports

Server messages are **text** (JSON) or **binary** (opaque bytes, e.g. MessagePack); each transport
below says how it tells them apart. Client messages are one JSON object each.

### `ws` — WebSocket

`GET /ws` upgrades to a WebSocket. Text frames carry text messages, binary frames binary ones.

### `sse` — Server-Sent Events + POST

```
GET  /sse/stream     opens the stream (Content-Type: text/event-stream)
POST /sse/send       body: one client message; header X-Indri-Connection-Id: <id>
```

The stream's first event names the connection; everything after it is a server message:

```
event: connected
data: <connection id: 64 hex chars>

data: {"stage":{"currentScene":"login"}}

event: binary
data: <base64>

: ping
```

A message containing newlines spans several `data:` lines, rejoined with `\n` per the SSE spec.
`: ping` comments arrive every `INDRI_WS_PING_PERIOD` seconds; ignore them.

`POST /sse/send` answers `204` after the message has been handled, so a client that waits for each
response before sending the next keeps its messages in order. `404` means the connection is unknown or
closed: reconnect. `413` means the body exceeded `INDRI_WS_MAX_MESSAGE_SIZE`.

**Treat the connection ID like the session token**: anyone holding it can send as that connection.

### `graphqlws` — GraphQL subscriptions

`GET /graphql` upgrades to a WebSocket that must negotiate the `graphql-transport-ws` subprotocol.
This is GraphQL *framing* only — the server executes no queries and ignores the query text.

```jsonc
→ {"type": "connection_init"}
← {"type": "connection_ack"}

// Opens the server→client stream; the connection counts as connected from here.
→ {"id": "events", "type": "subscribe",
   "payload": {"operationName": "IndriEvents", "query": "subscription IndriEvents { indriEvents }"}}
← {"id": "events", "type": "next", "payload": {"data": {"indriEvents": {"text": "{\"stage\":…}"}}}}
← {"id": "events", "type": "next", "payload": {"data": {"indriEvents": {"b64": "<base64>"}}}}

// Each client message is its own Send operation, completed once handled.
→ {"id": "s1", "type": "subscribe",
   "payload": {"operationName": "Send", "query": "mutation Send($message: String!) { send(message: $message) }",
               "variables": {"message": "{\"action\":\"refresh\"}"}}}
← {"id": "s1", "type": "complete"}
```

`ping` is answered with `pong`. A `Send` before `IndriEvents`, or an unknown operation, gets an
`error` message for that id. Completing `events` disconnects. Protocol violations close the socket:
`4400` invalid message, `4401` subscribe before `connection_ack`, `4406` subprotocol not offered,
`4408` no `connection_init` in time, `4409` subscription id already in use, `4429` duplicate
`connection_init`.

### `webrtc` — data channels

1. Create a peer connection, then an ordered data channel (the default), **then** the offer.
2. Wait for ICE gathering to complete — signaling is a single exchange, with no trickle ICE.
3. `POST /webrtc/offer` with the offer as `{"type": "offer", "sdp": "…"}`; the response is the
   answer in the same shape, candidates included. Apply it.

Once the data channel opens, string messages are text and binary messages are binary, both ways.
The offer endpoint answers `503` when `INDRI_WEBRTC_MAX_PEERS` is reached, and a peer whose data
channel hasn't opened within 15s of the answer is dropped.

### Load balancers

Messages for other players reach them on whichever instance they are connected to (with
`INDRI_LOCK_BACKEND=redis`), so players in one game need not share an instance.

- `ws` and `graphqlws` keep each client on one socket and need no affinity.
- `sse` splits a client across a stream and separate POSTs, so behind several instances it needs
  **sticky sessions by client IP** (there are no cookies to pin on). An SSE send that lands on the wrong
  instance gets `404`, and the client treats the connection as ended (automatic reconnect is not built
  yet; see `plans/session-resume.md`).
- `webrtc` needs no stickiness: the offer may reach any instance, and the answer carries that
  instance's own ICE candidates. Each instance must be reachable at the UDP address it advertises (its
  own `INDRI_WEBRTC_NAT_1TO1_IPS` and port range) — a load balancer in front of the UDP ports breaks it.

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
{ "action": "kick", "code": "my-room", "slotId": "<target's slot, e.g. p1>" }
```

Host-only. The caller is authorized from their own connection's session — never from the payload. The
target is removed from the game and, if connected (to any instance), sent `{"disconnected": true}` and
disconnected.

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

### Transport

Server→client messages are **MessagePack binary** by default, keyed by the same names as the JSON
forms shown here. Add `?debug=1` to the request that opens the connection to receive JSON text instead,
which is useful for inspection: the `/ws` or `/graphql` upgrade URL, the `GET /sse/stream` URL, or the
`POST /webrtc/offer` URL. With the client package, put it on `EXPO_PUBLIC_API_URL`. A WebSocket client
must set `binaryType = "arraybuffer"`.

### Layout frame

Carries the game's layout (its `data.layout`, seeded from the script and changed by the `layout`
action). Sent immediately before the keyframe on `create`, `join`, and `reconnect` (not on `refresh`),
and broadcast to the whole game whenever a write edits the layout.

```json
{ "o": 4, "v": "<16-char hex version>", "data": { "grid": { … }, "scenes": { … } } }
```

- `o: 4` is the layout opcode.
- `v` is the layout's version: a hash of its content, so it changes exactly when the layout does.
- `data` is the layout object itself. A client shows the newest layout it has received; a layout frame
  never changes game state, and deltas still decode against the keyframe.

### Slim keyframe wrapper

Follows the layout frame. `PrivateData` and the `layout` key are stripped from `data` before sending.

```json
{ "sv": "<same hash as v in layout frame>", "game": { … } }
```

- `sv` is the version of the game's layout when the keyframe was built (the `v` of its layout frame).
  Presence of `sv` identifies this message as a keyframe.
- `game` is a sanitized `models.Game` without `data.layout`. The client builds its positional map from
  `game` as sent, and shows the layout from the layout frame alongside it.

### Keyframes mid-game

Positional paths decode only against the schema of the keyframe the client holds. When a write adds or
removes an object key (the game's *shape* changes), the server sends every client in the game a fresh
keyframe instead of a delta; apply it exactly like the keyframe from `join`. Deleting a key therefore
always arrives as a keyframe; shrinking an array arrives as a delta with removed paths.

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

Debug mode changes only the encoding, not the paths: a debug connection receives the same integer
arrays, as JSON.

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
