# Indri client protocol

The same actions are reachable over three transports, which run side by side (see
[ARCHITECTURE.md](ARCHITECTURE.md#transports) — the `transport.Transport` abstraction):

| Transport | Client → server | Server → client |
|---|---|---|
| **WebSocket** | `ws://<host>:<port>/ws` — one JSON message per action | the same socket |
| **GraphQL** | `POST /graphql` typed mutations | `gameUpdates` subscription (graphql-transport-ws) |
| **REST + SSE** | `POST /api/<action>` JSON | `GET /events` event stream |

The default address is `localhost:5002`. All three drive the same connection-independent action logic
(`router.Dispatch`); only the wire framing and auth mechanism differ. The bulk of this document
describes the WebSocket message shapes; every action also exists as a GraphQL mutation and a REST route
with the same semantics.

The REST and SSE halves are two sides of one transport pair: SSE is push-only (it has no inbound
channel), so an SSE client sends its actions over REST — or over GraphQL mutations, which work equally
well. See [REST and SSE](#rest-and-sse) below.

Every WebSocket message is a JSON object. Client→server messages **must** carry a string `action` field;
the router uses it to pick handlers and removes it from the payload before the handler sees it. REST and
GraphQL name the action in the route or the mutation instead, so their payloads never carry it.

Every route is origin-checked by one shared policy (`transport.OriginPolicy`, applied in
`internal/entrypoints/http`). Requests with no `Origin` header (native apps, CLI tools,
server-to-server) are always allowed. A browser request is allowed if its origin appears in
`INDRI_ALLOWED_ORIGINS` — matched as an exact string, so scheme, host and port must all agree — or if
it is the server's own origin. An empty allowlist rejects all cross-origin browsers. Allowed
cross-origin requests get the matching CORS response headers, and preflights are answered centrally.

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

Marks the player disconnected in their game, **invalidates the session** (deletes it so its bearer
token can no longer be replayed via `reconnect`), and closes the connection. No-ops if the caller was
never authenticated.

### Pseudo-actions: `received` and `processed`

`router.Dispatch` runs handlers registered under the literal action `received` before, and `processed`
after, the message's real action. Registering a handler on either gives you a pre/post hook that fires
on every inbound message. Nothing is registered on them by default.

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

---

## GraphQL

`internal/transport/graphql` (gqlgen) mounts the same actions at `/graphql`. It runs alongside the
WebSocket transport and shares the broadcast pipeline, so WebSocket and GraphQL clients in the same game
receive identical deltas.

**Auth.** Mutations authenticate from the `Authorization: Bearer <token>` header; the subscription
authenticates from the graphql-transport-ws `connection_init` payload (`{"Authorization": "<token>"}`).
The token is the same secret returned by `login`/`reconnect`. `register`/`login`/`reconnect` need no
auth; the rest do.

**Dynamic data.** The `JSON` scalar (backed by `json.RawMessage`) carries the dynamic model blobs and
the delta/keyframe payloads verbatim — no double-encoding, objects and arrays both pass through.

### Mutations (client → server)

Each returns a `JSON` result — the same response document the WebSocket action produces.

| Mutation | Action | Notes |
|---|---|---|
| `register(email, password, name)` | register | |
| `login(email, password)` | login | result carries the bearer token in `sessionId` |
| `reconnect(token)` | reconnect | |
| `createGame(code, teamId, private)` | create | |
| `joinGame(code, teamId)` | join | |
| `leaveGame` | leave | |
| `kick(code, userId)` | kick | host-only |
| `logout` | logout | |
| `inquire(inquiryType, inquiry, code)` | inquire | |

```graphql
mutation { login(email: "u@e.io", password: "…") }        # returns JSON incl. sessionId token
mutation { createGame(code: "my-room", teamId: "team1", private: false) }   # Authorization header required
```

Unauthenticated authed-only mutations return a GraphQL error (`"not authenticated"`).

### Subscription (server → client)

```graphql
subscription { gameUpdates(gameId: "<game object id>") }
```

Each event is a game **delta** — the same document the WebSocket delta path emits (see
[Delta](#delta--change-event)), sanitized identically. Reconnecting into an active game over GraphQL:
call the `reconnect` mutation for the auth payload, then subscribe to `gameUpdates` for state.

---

## REST and SSE

`internal/transport/rest` and `internal/transport/sse` are the third way to play, and the only one that
needs no persistent socket. They are a pair: SSE carries server→client pushes and has no inbound
channel, so actions go over REST.

Both share the broadcast pipeline with WebSocket and GraphQL, so clients on all three transports in the
same game receive identical deltas.

### Actions — `POST /api/<action>`

The route name *is* the action name. The body is a JSON object of the action's arguments; an empty body
is treated as `{}`, so argument-less actions can be POSTed with nothing.

| Route | Action | Body |
|---|---|---|
| `POST /api/register` | register | `{email, password, name}` |
| `POST /api/login` | login | `{email, password}` — result carries the bearer token in `sessionId` |
| `POST /api/reconnect` | reconnect | `{token}` |
| `POST /api/create` | create | `{code, teamId, private?}` |
| `POST /api/join` | join | `{code, teamId}` |
| `POST /api/leave` | leave | — |
| `POST /api/kick` | kick | `{code, userId}` — host-only |
| `POST /api/logout` | logout | — |
| `POST /api/inquire` | inquire | `{inquiryType, inquiry?, code?}` |
| `POST /api/refresh` | refresh | — returns the current keyframe |

**Auth.** `Authorization: Bearer <token>`, the same secret `login`/`reconnect` return.
`register`/`login`/`reconnect` need none; the rest do. The caller is always resolved from their own
token, never from a body field.

**Responses.** `200` with the action's response document, or `{}` for an action whose only effect is a
broadcast. `400` for a missing or mistyped argument and for a handler error, `404` for an unknown
action, `405` for a non-POST, each as `{"error": "…"}`.

Arguments are validated before dispatch and unknown body keys are dropped, so a caller cannot smuggle
extra fields into a payload. `TestRestRoutesMatchRegisteredActions` fails if this table drifts from the
handler registry in either direction.

### Stream — `GET /events`

```
GET /events?token=<session token>
```

**Auth.** `Authorization: Bearer <token>` if you can set headers. The browser `EventSource` API cannot,
so the `token` query parameter is the only option there — it is a first-class part of the protocol, not
a fallback. Either way the token is the one `login`/`reconnect` returned. An unknown token is `401`.

Each event is a game **delta**, one per `data:` frame — the same document the WebSocket delta path emits
(see [Delta](#delta--change-event)), sanitized identically:

```
data: {"id":"…","op":"update","ts":"…","type":"game","updated":{"players.…":{…}}}

```

A payload containing newlines is emitted as consecutive `data:` lines of one event, per the SSE spec.
An idle stream emits a `:ping` comment frame every 25 seconds so proxies do not drop it.

A subscriber that stops reading is not allowed to stall the broadcaster: once 16 deltas are queued the
oldest are dropped, and the client recovers with `POST /api/refresh`.

**Bootstrapping.** SSE delivers only deltas, so a client cannot build state from the stream alone:

1. `POST /api/login` (or `/api/reconnect`) → bearer token
2. `POST /api/join` or `/api/create` → the keyframe, in the response
3. `GET /events?token=…` → open the stream
4. `POST /api/refresh` → a fresh keyframe if the stream ever drops or a delta is missed

Open the stream before or immediately after joining. Deltas broadcast while no stream is open are not
replayed — recover with `refresh`.
