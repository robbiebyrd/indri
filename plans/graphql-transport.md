---
title: GraphQL transport (subscriptions + typed mutations)
status: in_progress
created: "2026-09-09"
---

# GraphQL transport

Add GraphQL as a first-class client protocol alongside WebSocket, using the
`internal/transport` abstraction. Decision (confirmed): **gqlgen**, dynamic
blobs (`data`/`privateData`/`playerData`, scene data) and the subscription
delta payload as a custom **JSON scalar** bound to `map[string]interface{}`;
typed **mutations** for the receive path; **subscription** for the transmit
path. Receive is refactored to be connection-independent (token-authed),
per the approved decision.

## Why the receive refactor

GraphQL splits what the transport currently bundles: clients **send via
mutations** (stateless HTTP, token-authed) and **receive via a subscription**
(persistent server→client). Handlers today take a live `transport.Conn` and
write responses to it. To serve both WS and GraphQL from one code path, action
logic must not depend on the inbound socket: it takes the authenticated session
+ payload and **returns** a result.

## Target shape

Inbound (both transports feed this):

```go
// package actions
type Request struct {
    Session *models.Session          // nil until authenticated
    Payload map[string]interface{}
}
type Result struct {
    Responses        [][]byte         // messages to return to the caller
    Session          *models.Session  // set when this call establishes/refreshes auth
    DisconnectIDs    []string         // sessionIds to force-disconnect (kick/logout)
}
type MessageHandler interface { Handle(Request) (Result, error) }
```

- login/register/reconnect: `Session == nil` in; return `Responses` (+ `Session`
  for login/reconnect so the transport binds its channel and the token is
  returned to the caller).
- create/join/leave/refresh/inquire: `Session` required.
- kick/logout: return `DisconnectIDs`; the transport closes those conns, which
  triggers the normal disconnect cleanup.

Outbound stays on the existing abstraction: a **MultiTransport** aggregates the
WS and GraphQL transports so `broadcast` fans out to both; each transmit conn
carries its `sessionId` key (WS sets it on login; a GraphQL subscription sets it
from the `connection_init` token). `events.Publisher` and `broadcast` are
unchanged apart from targeting the Multi.

## Increments

1. **Conn-independent dispatch (foundation).** New `actions.Request/Result` +
   `MessageHandler.Handle(Request)`. A `dispatch.Dispatcher` resolves the
   session and runs the action lifecycle (`received`/action/`processed`).
   Refactor all handlers + the example move handler. The WS transport's message
   loop: resolve session from the socket's `sessionId` key → Dispatch → write
   responses, bind session on auth, close DisconnectIDs. **WS behavior
   unchanged; verified end to end.**
2. **MultiTransport.** `transport.Multi([]Transport)` implementing the interface
   by delegating/aggregating. Injector builds `Multi{ws, graphql}`.
3. **gqlgen.** `gqlgen.yml` + `schema.graphqls` (JSON scalar, `Mutation` per
   action, `Subscription { gameUpdates(gameId): JSON }`). Mutation resolvers →
   Dispatcher (token from `Authorization`); subscription resolver registers a
   `transport.Conn` (Write pushes a `next` event) keyed by session. GraphQL
   transport implements `transport.Transport`; `Register` mounts `/graphql`
   (POST + graphql-ws). Verify GraphQL send+receive end to end.

## Non-goals (now)
- Per-game-script typed schema (blobs stay JSON scalars — introspection stays
  stable).
- Removing WebSocket (both run side by side).
