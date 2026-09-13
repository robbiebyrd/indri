# Indri client protocol

The same actions are reachable over four transports, which run side by side (see
[ARCHITECTURE.md](ARCHITECTURE.md#transports) — the `transport.Transport` abstraction):

| Transport | Client → server | Server → client |
|---|---|---|
| **WebSocket** | `ws://<host>:<port>/ws` — one JSON message per action | the same socket |
| **GraphQL** | `POST /graphql` typed mutations | `gameUpdates` subscription (graphql-transport-ws) |
| **REST + SSE** | `POST /api/<action>` JSON | `GET /events` event stream |
| **WebRTC** | `game` DataChannel — one JSON message per action | the same DataChannel |

The default address is `localhost:5002`. All four drive the same connection-independent action logic
(`router.Dispatch`); only the wire framing and auth mechanism differ. The bulk of this document
describes the WebSocket message shapes; every action also exists as a GraphQL mutation and a REST route
with the same semantics, and WebRTC carries the WebSocket shapes verbatim.

The REST and SSE halves are two sides of one transport pair: SSE is push-only (it has no inbound
channel), so an SSE client sends its actions over REST — or over GraphQL mutations, which work equally
well. See [REST and SSE](#rest-and-sse) below.

WebRTC is the odd one out in a useful way: a DataChannel is bidirectional and message-oriented, so it
carries exactly the WebSocket message shapes in both directions and needs no separate inbound surface.
See [WebRTC](#webrtc) below.

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

### `layout`

```json
{ "action": "layout", "code": "my-room", "op": "addWidget",
  "sceneId": "board", "widgetId": "title",
  "widget": { "type": "text",
              "placement": { "kind": "grid", "col": 0, "row": 0, "w": 12, "h": 2 },
              "config": { "text": "Tic Tac Toe" } } }
```

Host-only. Edits `game.data.layout`, the board that every player in the game renders (see
[ARCHITECTURE.md](ARCHITECTURE.md#layout-engine)). The caller is resolved from their own authenticated
session. A `userId` in the payload is never read as the caller — payload fields are only ever the *subject*
of an edit.

`code` and `op` are required on every message. One action carries all seven operations behind the `op`
discriminator; `op` selects which other fields the message may carry.

| `op` | Required fields | Effect |
|---|---|---|
| `addWidget` | `sceneId`, `widgetId`, `widget` | Inserts a widget. Creates the scene if it does not exist yet — there is no `addScene` op. An id that is already in use is an error, not a replace. |
| `removeWidget` | `sceneId`, `widgetId` | Deletes the widget. A missing scene or a missing widget is a no-op, so a client can retry after a reconnect. |
| `setPlacement` | `sceneId`, `widgetId`, `placement` | Replaces the whole placement. This is **both move and resize**: they write the same field, so there is no separate resize op. A partial placement is not a placement. |
| `setWidgetConfig` | `sceneId`, `widgetId`, `config` | Merges into the widget's config, so a panel can send one field at a time. Deleting a config key is not expressible; remove and re-add the widget. |
| `setStyle` | `scope`, `style` | Replaces the style of the board, a scene, or a widget. |
| `setGrid` | `grid` | Sets the board grid, `{cols, rows}`. Not scoped. A sub-grid's own grid lives in its config — change it with `setWidgetConfig`. |
| `setScript` | `scope`, `source` | Sets the Lua script of the board, a scene, or a widget. An empty `source` deletes the field rather than storing `""`. |

`setStyle` and `setScript` name their target with `scope`. The scope decides which address fields the
message must carry. An address a scope does not use is rejected, not ignored — ignoring it would edit a
different target than the client meant.

| `scope` | `sceneId` | `widgetId` |
|---|---|---|
| `board` | must **not** be set | must **not** be set |
| `scene` | required | must **not** be set |
| `widget` | required | required |

Unknown keys are rejected **per op**, so a field that one op accepts is an error on another: `widget` is
required by `addWidget` and rejected by `setPlacement`. Only `code` and `op` are accepted by every op.

**There is no reply.** The action returns no response document. The edit reaches every player in the game —
the editing host included — as a broadcast [delta](#delta--change-event). A rejected op writes nothing and
returns an error. An op that changes nothing commits nothing and publishes nothing, so no empty delta occurs.

**Validation.** The op is applied to a detached copy, and the whole resulting layout is then validated. Only
a copy that passes is stored, so a rejected op leaves the game exactly as it was. Errors name the dotted path
of the offending value, for example `scenes.board.widgets.title.placement`. The limits are:

| Limit | Value |
|---|---|
| Widgets per layout | 300, counted across all scenes and all nested sub-grids |
| Serialised layout size | 256 KiB |
| Grid dimensions (`cols`, `rows`, at every level) | 8 to 4096 |
| Sub-grid nesting | 4 grid levels, counting the scene's own grid as level 1 — at most three nested sub-grids |

A layout must have a `grid`, so the first op on a game with no layout must be `setGrid`. Grid-placed widgets
may not overlap their siblings at the same level; edge-touching is not an overlap. Absolute placements are
exempt from the overlap rule, and their offsets are percentage strings only (`"25%"`) — pixel values are
rejected. `privateData` is rejected anywhere in a layout at any depth (see
[Data model](ARCHITECTURE.md#data-model)).

A widget's `config` and `style` contents stay opaque to the server. Indri is a framework and does not know
what a "text" widget is, so it validates structure and not widget semantics.

The action is reachable on all three transports: as the WebSocket message above, as the `layout`
[GraphQL mutation](#graphql), and as `POST /api/layout` ([REST](#rest-and-sse)).

> **Deployment warning.** A host can set arbitrary media URIs and arbitrary Lua source, and the server
> delivers both to every player in the game, where the client renders and executes them. See
> [Host-authored content](ARCHITECTURE.md#host-authored-content--accepted-risk).

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

A path segment is one document key, and a key is arbitrary — a user id, a team id, a widget id, a
script table key. A key holding a `.` or a `\` therefore has those characters escaped with a
backslash, so `widgets.foo\.privateData.label` addresses the `label` of the widget whose id is
literally `foo.privateData`. **Split a path on unescaped dots only** and unescape each segment; a path
with no backslash splits exactly as a plain split on `.` would.

Deltas are sanitized on the same terms as keyframes: paths with a segment that *decodes to*
`privateData` are dropped, and `privateData` is stripped out of whole-object update values. A client
never sees private data on either path. Because the comparison is on decoded segments, a key merely
named `foo.privateData` is an ordinary key and is broadcast normally.

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
| `layout(code, op, args)` | layout | host-only; result is always null |

An op's own fields ride in `args` because their shape depends on the op, which the GraphQL type system
cannot express. The resolver flattens `args` into the payload, then sets `code` and `op` last — so an `args`
that carries its own `code` or `op` cannot redirect the edit. There is no `refresh` mutation; use the REST
route or the WebSocket action for a keyframe.

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
| `POST /api/layout` | layout | `{code, op, …the op's own fields}` — host-only |

**Auth.** `Authorization: Bearer <token>`, the same secret `login`/`reconnect` return.
`register`/`login`/`reconnect` need none; the rest do. The caller is always resolved from their own
token, never from a body field.

**Responses.** `200` with the action's response document, or `{}` for an action whose only effect is a
broadcast. `400` for a missing or mistyped argument and for a handler error, `404` for an unknown
action, `405` for a non-POST, each as `{"error": "…"}`.

Arguments are validated before dispatch and unknown body keys are dropped, so a caller cannot smuggle
extra fields into a payload. `layout` is the one exception, because an op's arguments are objects whose
shape depends on the op: its body passes through whole, and the handler rejects every field the op does not
declare. `TestRestRoutesMatchRegisteredActions` fails if this table drifts from the handler registry in
either direction.

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

---

## WebRTC

`internal/transport/webrtc` carries the protocol over a WebRTC DataChannel. Unlike SSE it is
bidirectional, so it is shaped like the WebSocket transport rather than the push-only ones: the same
JSON messages travel in both directions, with the same `action` field, and the same server → client
keyframes and deltas come back. Nothing in [Client → server](#client--server) or
[Server → client](#server--client) changes.

It shares the broadcast pipeline with the other three, so clients on all four transports in the same
game receive identical deltas.

### Connecting

Signalling is one HTTP round trip. The server answers **non-trickle** — a single complete SDP with its
ICE candidates already gathered — and there is no channel for streaming candidates afterwards, so the
client must finish gathering before it posts:

```
POST /rtc/offer
{ "type": "offer", "sdp": "<complete SDP, candidates gathered>" }

200 OK
{ "type": "answer", "sdp": "<complete SDP, candidates gathered>" }
```

Create both DataChannels **before** the offer, so they are negotiated in it:

| Label | Purpose |
|---|---|
| `game` | client actions and server keyframes/deltas — reliable and ordered |
| `signal` | renegotiation only; never reaches the action router |

`game` must be created with no `RTCDataChannelInit`. Reliable and ordered is the DataChannel default,
and that is exactly what makes it match Indri's delta-delivery contract — an unreliable channel would
silently weaken it.

### Auth

`Authorization: Bearer <token>` on the signalling POST binds the session at handshake, as SSE does.

It is **optional**. With no token the connection is anonymous and carries no session key, and a
`login` or `reconnect` sent over the `game` DataChannel binds one through the ordinary dispatch path —
exactly as it does over WebSocket. This is what lets WebRTC stand alone: a client can register, log in
and play without any other transport ever being open.

### Limits

Anonymous signalling means an unauthenticated caller can allocate PeerConnections in a loop, so two
limits are enforced, both configurable (`INDRI_RTC_MAX_PEERS`, `INDRI_RTC_PENDING_TTL`):

- a global peer cap — once reached, `POST /rtc/offer` returns **503** without allocating anything
- a pending TTL — a peer that never reaches `connected` is closed and dropped

Payloads above **16 KiB** are logged and dropped rather than sent. That is the practical cross-browser
DataChannel ceiling, not the spec's 256 KiB. Chunking is not implemented, so a keyframe larger than
that will not arrive — recover with `refresh`.

A kicked player's DataChannel stops carrying traffic in both directions immediately, but the underlying
PeerConnection is reclaimed only when it fails or its TTL expires.

### Deployment

`INDRI_RTC_UDP_PORT` (default 8443) is the single UDP port every peer shares. It must be reachable, and
it must not be remapped: the ICE mux advertises the port it binds verbatim in its candidates, so a
container mapping like `9000:8443` advertises an address nothing can reach. `docker compose --profile
server up -d` publishes it correctly.

Behind a NAT, set `INDRI_RTC_NAT_1TO1_IPS` to the externally routable address, or ICE gathering
advertises a private one. A misconfiguration here fails *silently at the HTTP layer*: the signalling
POST returns a perfectly normal 200 and the DataChannel simply never opens.

**TURN is required in production even though no media is involved.** NAT traversal applies to a
DataChannel exactly as it does to audio or video, and a meaningful share of consumer connections cannot
be established without a relay. `INDRI_RTC_ICE_SERVERS` configures it.
