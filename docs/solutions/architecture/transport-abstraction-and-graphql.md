---
type: decision
tags: [architecture, convention, transport, graphql, websocket]
created: "2026-09-09"
---

# Swappable client transports + GraphQL

**Context:** The server was hard-coupled to melody/WebSocket — `*melody.Session`
was threaded through every handler, the router, connection/broadcast services,
and boot. We needed to add protocols (GraphQL subscriptions, and later REST+SSE,
WebRTC data channels, WebTransport) without rewriting the app each time.

**Decision:**

1. **`internal/transport` abstraction.** Two interfaces:
   - `Conn` — one client connection: `Get/Set/UnSet(key)`, `Write`, `Close`,
     `IsClosed`.
   - `Transport` — the hub: `Handle(Handlers{Connect,Disconnect,Message,Error})`,
     `Register(mux)` (each transport owns its HTTP routes), `Broadcast`,
     `BroadcastFilter`, `Conns`, `Close`, `IsClosed`.

   WebSocket is one adapter (`internal/transport/ws`, the ONLY place melody is
   imported). `transport.Multi` aggregates several transports so `broadcast`
   fans out to all of them at once; each push conn carries its `sessionId` key,
   which is how broadcast targets it.

2. **Connection-independent dispatch (receive path).** Action handlers do NOT
   take a `Conn`. They implement `actions.MessageHandler.Handle(actions.Request)
   (actions.Result, error)` where `Request{Session, Payload}` and
   `Result{Responses, Session, DisconnectIDs}`. `router.Dispatch(session,
   action, payload)` is the single entry point; a transport resolves the session
   (WS: from the socket's `sessionId` key; GraphQL: from the bearer token) and
   applies the result (write responses, bind session on auth, force-close
   DisconnectIDs). login/reconnect return the `Session`; kick/logout return
   `DisconnectIDs`.

3. **GraphQL (`internal/transport/graphql`, gqlgen).** Mounted at `/graphql`
   (POST + graphql-transport-ws), alongside `/ws`. Typed **mutations** dispatch
   to `router.Dispatch` (auth from the `Authorization` header on POST, or the
   `connection_init` payload on WS). A **`gameUpdates` subscription** registers a
   channel-backed `transport.Conn` (keyed by session) that the existing
   broadcast writes to — no change to broadcast or the events.Publisher. Dynamic
   model data and delta payloads use a custom `JSON` scalar bound to
   `json.RawMessage` (no double-encoding; objects and arrays both pass through).

**Consequences:**
- Adding a protocol = implement `transport.Transport` (+ a `Conn`) and add it to
  the `Multi` in the injector. Nothing above the interface changes.
- Handlers are transport-agnostic and unit-testable without a socket.
- The session bearer token (not connection state) is the source of auth, which
  is what makes stateless GraphQL mutations and the subscription's
  `connection_init` auth work uniformly.
- Do NOT reach for `*melody.Session` outside `internal/transport/ws`, and do NOT
  make handlers write to a connection — return `Responses` instead.
