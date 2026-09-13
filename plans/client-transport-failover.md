---
status: in_progress
approved_at: "2026-09-13T23:20:21.337Z"
updated: "2026-09-13T23:22:21.043Z"
started_at: "2026-09-13T23:22:21.043Z"
---
# Plan: Pluggable client transports with automatic failover

**Created:** 2026-09-13 | **Status:** Draft | **Effort:** L | **Branch:** `POC-00003/client-transport-failover`

## Summary

Let the client speak to the server over whichever transport works, picked automatically and switched
automatically when one dies, with the app never knowing which is in use. Exactly one channel is live at
a time: WebRTC, then WebSocket, then REST+SSE, in preference order.

The whole design turns on one trick: **the supervisor itself implements `ClientTransport`**. So
`MessageHandler` keeps taking a single transport and does not change, and transparency is structural
rather than something callers must cooperate with.

## Architecture Context

- `ClientTransport` (`client/services/transport.ts`) already exists, extracted in `09d6c48`, with
  `WebSocketTransport` and `WebRTCTransport` implementing it. This plan adds a third implementation and
  a supervisor that composes them.
- `MessageHandler` takes one `ClientTransport` as an optional final constructor argument and defaults to
  `WebSocketTransport`. It is constructed exactly once, in `SocketProvider`, at the root of the app —
  deliberately, because the server session is per-connection and a socket owned by a route dies with it.
- **The server session is per connection, but `reconnect` rebinds it** from the stored bearer token, on
  *any* transport. That is the mechanism that makes switching channels possible without losing identity
  or game membership. `SocketProvider` already does this on open; failover reuses that path rather than
  inventing one.
- **`GameStateParser` is keyframe + ordered-delta replay** and discards deltas older than the keyframe's
  cutoff. So a gap after a switch is closed by `refresh`, and a duplicate delta is harmless. Most of what
  a failover design normally has to invent is already here.
- All four server transports carry identical messages into `router.Dispatch`, so a switch changes the
  pipe and nothing else.

ADR: **the supervisor implements the transport interface** rather than `MessageHandler` learning to
hold several. Rejected a transport-aware `MessageHandler`: it would put connection policy next to
message parsing, and every consumer would grow a notion of "which channel".

ADR: **queued sends do not survive a switch.** The server has no idempotency keys, so replaying a
queued `create` fails and a replayed game action double-applies. The queue covers the pre-open window on
one channel only; on a switch it is dropped and the app resyncs. Rejected replay-on-reconnect, which
trades a visible dropped action for an invisible duplicated one.

## Research Findings

- `ClientTransport.onMessage`/`onOpen` are **setters, not subscriptions** (`ws.onmessage = ...`), so only
  one sink can exist. The supervisor needs to attach and detach per channel, so these must return an
  unsubscribe.
- There is **no failure signal on the interface**. `onerror`/`onclose` are swallowed inside
  `WebSocketTransport` behind `//TODO: Handle reconnects`. Nothing can currently trigger a failover.
- **`send` drops silently before open** and has no queue.
- **SSE requires a token at handshake** (`?token=`), while WebRTC and WebSocket accept anonymous
  connections and bind a session when `login` arrives. REST+SSE therefore **cannot be the first channel
  for a logged-out user** — the candidate list is dynamic, not a fixed array.
- **No SSE or REST client transport exists yet.** Only `WebSocketTransport` and `WebRTCTransport`.
- `MessageHandler.onOpen` latches on `opened`, firing observers once. Session restore must run on *every*
  connect, so the latch has to become "run now if connected, and on every future reconnect".
- `webrtc-signal.ts` already derives the signalling URL from the `ws://` base. That logic generalises to
  every endpoint and should move rather than be copied.
- The client's only test runner is `node --experimental-strip-types`, which has neither WebSocket nor
  WebRTC. The supervisor must therefore be testable against fake transports — which it is, since its
  logic is pure policy.

## Security Considerations

- The SSE token travels in a query string, so it lands in server logs and browser history in a way an
  `Authorization` header does not. That is forced by `EventSource` and already true server-side; the
  client should not widen it by logging the URL.
- A failover must not downgrade auth: the stored bearer token is replayed over the new channel, so the
  new connection must be established before the old one's session is considered gone.
- Channel-change diagnostics must not log the token or the full SSE URL.

## Performance Considerations

- Probing costs a connection attempt per candidate. Bound each attempt (a few seconds) so a dead channel
  cannot stall startup, and remember the last working channel so the common case is one attempt.
- Backoff between cycles, or a server outage turns every client into a retry loop across three channels.
- A switch costs `reconnect` + `refresh`, i.e. one full keyframe. That is the price of correctness; it
  should not fire more often than the backoff allows.

## Open Questions

### Critical (P1 - Blockers)
1. Should a working-but-less-preferred channel be upgraded back to a preferred one later (WebSocket
   established, WebRTC becomes reachable)? — **Default if unanswered: no.** Stay on what works until it
   fails. Upgrading costs a keyframe and risks flapping.

### Important (P2 - Affects implementation)
2. Should the app be able to *see* the active channel (a debug indicator)? Transparent means it need
   not, but not knowing makes support harder. Default: expose an observer, use it nowhere by default.

## Steps

### Step 1: Extend the transport contract
- **Test:** `client/services/transport.node-test.ts` — `onMessage`/`onOpen` return working unsubscribes;
  `onClose` fires with a reason on a closed socket; `WebSocketTransport` still drops send-before-open
  with its warning.
- **Implement:** `client/services/transport.ts`
- **Code:**
  ```ts
  export interface ClientTransport {
      readonly name: string            // for diagnostics, never a branch target
      connect(url: string): Promise<void>   // resolves on open, rejects on failure/timeout
      send(message: object): void
      onMessage(handler: (data: string) => void): () => void
      onOpen(handler: () => void): () => void
      onClose(handler: (reason: string) => void): () => void  // the failover trigger
      close(): void
  }
  ```
- **Constraint:** behaviour-preserving for `WebSocketTransport`; `MessageHandler` must still work with it
  directly, so the existing tests stay green unchanged.
- **Validation:** `cd client && pnpm run typecheck && pnpm test`

### Step 2: One place that derives endpoints
- **Test:** `client/services/endpoints.node-test.ts` — `ws://h:p/ws` yields the WebRTC, SSE and REST URLs;
  `wss://` maps to `https://`; a URL without the `/ws` suffix still works.
- **Implement:** `client/services/endpoints.ts`, moving the derivation out of `webrtc-signal.ts`
- **Code:**
  ```ts
  // EXPO_PUBLIC_API_URL is a ws:// URL because WebSocket was the only transport.
  // Every other channel hangs off the same origin, so derive rather than add config.
  export function endpoints(apiUrl: string): {
      websocket: string; rtcOffer: string; events: string; api: string
  }
  ```
- **Depends on:** Step 1
- **Validation:** `cd client && pnpm test`

### Step 3: SSE + REST transport
- **Test:** `client/services/sse-transport.node-test.ts` — pure parts only: the action-to-route mapping,
  and that `connect` rejects without a token. The stream itself needs a runtime the test runner lacks.
- **Implement:** `client/services/sse-transport.ts`
- **Code:**
  ```ts
  // Push-only inbound over EventSource, outbound over POST /api/<action>.
  // Requires a token at handshake: EventSource cannot set headers, so the
  // server takes it as a query parameter. That is why this channel is not
  // eligible until a session exists -- see the candidate list in Step 4.
  export class SseRestTransport implements ClientTransport {
      constructor(private token: () => string | null) {}
  }
  ```
- **Constraint:** never log the stream URL — it carries the bearer token.
- **Depends on:** Step 2
- **Validation:** `cd client && pnpm run typecheck && pnpm test`

### Step 4: The failover supervisor
- **Test:** `client/services/failover-transport.node-test.ts` — against fake transports: picks the first
  that connects; falls through when the first rejects; switches when the active one closes; emits one
  `onOpen` per successful connect; a send before any channel is open is queued and flushed; a queued send
  is **dropped, not replayed**, when the channel changes; `close()` stops all probing and does not
  reconnect.
- **Implement:** `client/services/failover-transport.ts`
- **Code:**
  ```ts
  // Implements ClientTransport, so MessageHandler is unchanged and the rest of
  // the app cannot tell which channel it is on. That is the whole design.
  export class FailoverTransport implements ClientTransport {
      readonly name = "failover"
      constructor(
          private readonly candidates: () => ClientTransport[],  // re-evaluated per cycle:
                                                                 // SSE is ineligible until a token exists
          private readonly opts: {attemptTimeoutMs: number; backoffMs: number; queueLimit: number},
      ) {}
  }
  ```
- **Constraint:** the queue is bounded and drops oldest with a warning, matching the server's own
  drop-oldest policy rather than inventing a second one.
- **Depends on:** Step 3
- **Validation:** `cd client && pnpm test`

### Step 5: Reconnect on every connect, not just the first
- **Test:** `client/services/message-handler.node-test.ts` — an `onOpen` observer runs immediately when
  already connected AND again on a later reconnect; an observer that throws does not stop the next.
- **Implement:** `client/services/message-handler.ts`
- **Code:**
  ```ts
  // The latch used to fire observers once. Session restore has to run on EVERY
  // connect: a failover lands on a fresh server-side connection with no
  // identity, and without re-running this the player is silently logged out
  // mid-game.
  ```
- **Constraint:** behaviour-preserving for the first-load case, which the existing tests cover.
- **Depends on:** Step 4
- **Validation:** `cd client && pnpm test`

### Step 6: Wire it in, and resync after a switch
- **Test:** `client/services/failover-transport.node-test.ts` — after a switch, `reconnect` is sent before
  any queued application message, and a `refresh` follows.
- **Implement:** `client/providers/socket/socket-provider.tsx`
- **Code:**
  ```tsx
  const transport = new FailoverTransport(() => [
      new WebRTCTransport(),
      new WebSocketTransport(),
      ...(token ? [new SseRestTransport(() => token)] : []),  // SSE needs a session
  ], {attemptTimeoutMs: 5000, backoffMs: 1000, queueLimit: 32})
  ```
  On each connect: `reconnect` with the stored token, then `refresh` — `refresh` because a switch lands
  on a new connection whose deltas start mid-stream, and the parser needs a keyframe to replay onto.
- **Depends on:** Step 5
- **Validation:** `cd client && pnpm run typecheck && pnpm test`, then manually kill each channel in turn
  against a live server and confirm play continues.

### Step 7: Documentation
- **Implement:** `docs/PROTOCOL.md`, `README.md`
- Document that a client may arrive on any transport and switch mid-session, that `reconnect` + `refresh`
  is the resync contract, and that an action in flight during a switch may be lost — which is why it is
  dropped rather than replayed.
- **Depends on:** Step 6

## Acceptance Criteria

- [x] The app runs unchanged: `MessageHandler`'s public surface is the same
- [x] With all channels healthy, WebRTC is selected
- [x] Killing the active channel moves play to the next one without a reload
- [x] Identity and game membership survive a switch, via `reconnect` + `refresh`
- [x] A send before any channel is open is queued and delivered
- [x] A send queued across a switch is dropped with a warning, never replayed
- [x] SSE is not attempted while logged out
- [x] `close()` stops probing and does not resurrect a channel
- [x] No bearer token appears in any log line
- [x] `pnpm run typecheck`, `pnpm test`, `pnpm run lint` clean

## Checklist (non-TDD cleanup)

- [x] `webrtc-signal.ts` no longer derives URLs itself — `signalUrl` deleted, `endpoints()` is the one derivation
- [x] `//TODO: Handle reconnects` in `transport.ts` removed, since this plan is that work
- [x] Verify CI-style: `rm -rf node_modules && pnpm install --frozen-lockfile && pnpm run typecheck && pnpm test` — 396 tests pass, typecheck clean, lint 0 errors
