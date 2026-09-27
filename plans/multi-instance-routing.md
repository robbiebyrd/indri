# Multi-instance routing and sticky sessions

Status: **proposal — options for Boss to choose from.** Nothing here is built.

## Where we are (line refs are for `wip/transports-compact-delta`)

**What each transport needs from a load balancer:**

| Transport | In-memory state | Requests that must reach the same instance |
|---|---|---|
| `ws` | melody session | none: one socket (`GET /ws`) |
| `graphqlws` | socket and subscription state | none: one socket; `Send` comes in on it |
| `sse` | `Hub` entry keyed by a random 256-bit connection ID (`sse.go:76,87`) | **every `POST /sse/send`**; an unknown ID gets `404` (`sse.go:144-147`) |
| `webrtc` | pion peer (`webrtc.go:186-194`) | only the one `POST /webrtc/offer`; the rest is **UDP to the addresses in the answer** (no trickle ICE) |

**Cross-instance fan-out works only for deltas.**
- With `INDRI_LOCK_BACKEND=redis`, every store write publishes to `indri:changes`.
- Every instance subscribes, and each writes to its own local connections (`monitor.go`,
  `broadcast.go:133-158`).

**Paths that skip the bus and silently reach only the local instance.** These are bugs *regardless of
stickiness*, because two players in one game can be connected to different instances.
1. **Lua `indri.refreshAll`** (`luahandler/handler.go:56`) calls `BroadcastService.Broadcast`
   directly.
2. **Kick** finds the target's connection by scanning local `Conns()` (`connection.go:75-89`). On
   another instance the target is removed from the game, but their connection is never closed and never
   sent `{"disconnected": true}`.
3. **Targeted sends** (`sendToTeam`, `sendToPlayer`, `sendToPlayers`) are local-only. They have no
   callers today, but they're public API that silently fails with several instances.

**Docs that are wrong today:**
- `docs/PROTOCOL.md:103` and `plans/pluggable-transports.md` say an SSE send to the wrong instance
  "gets `404` and reconnects". The client ends the connection instead (`sse.ts:114-115`).
- `docs/ARCHITECTURE.md:37` says broadcasts go through `Transport.BroadcastFilter`; the code uses
  `Conns()`.
- The docs say WebRTC needs IP-hash stickiness. It doesn't: the offer POST can land on any instance,
  and the answer carries *that* instance's candidates. What WebRTC needs is **each instance reachable
  at the UDP address it advertises** (per-instance `WEBRTC_NAT_1TO1_IPS` and port range, or a per-node
  public IP). An L4 load balancer in front of the UDP ports would break it.

## Part 1: required whatever we choose for stickiness

**Route every game-scoped send through the event bus.**
- Add bus event kinds beside the existing change events:
  - **Broadcast:** `{gameId, teamId?, sessionIds?, payload}`. Every instance delivers to its local
    connections, using the same `broadcastToSessions` filter as today.
  - **Close sessions:** `{sessionIds, reason}`. Every instance closes its matching local connections
    with `{"disconnected": true, reason}`. Used by kick, and by the "last connection wins" rule in
    `plans/session-resume.md`.
- `BroadcastService` publishes instead of writing, so every current and future caller works across
  instances. Only replies on the caller's *own* connection (`refreshSelf`, handler responses,
  `HandleConnect`) stay direct: that connection is local by definition.
- With the in-process bus (single instance) behavior is unchanged. The bus just loops back.
- **Tests:**
  - Two injectors sharing one Redis bus (or two in-process "instances" sharing a fake bus) prove that a
    kick and a `refreshAll` on instance A reach a connection on instance B.
  - Every existing broadcast test keeps passing.

Also fix the three doc errors above.

## Part 2: SSE affinity (the only transport that truly needs it)

### Option 1: load-balancer affinity (current plan, made graceful)

Keep "sticky by client IP", document LB examples (nginx `ip_hash`, HAProxy `balance source`), and
rely on `plans/session-resume.md` so a wrong-instance `404` becomes an automatic reconnect plus
resume instead of a dead client.
- **Pros:** no server code beyond resume; works on any L7 load balancer that supports source-IP
  affinity.
- **Cons:**
  - Client IP is a weak key: carrier-grade NAT and offices crowd many clients onto one instance, and a
    mobile client that switches Wi-Fi to cellular changes IP (it loses the stream anyway, and resume
    covers it).
  - Some managed load balancers offer only cookie affinity, and SSE over `fetch` sends cookies
    cross-origin only with `credentials: 'include'` plus `Access-Control-Allow-Credentials`.

### Option 2: cookie affinity

The same as option 1, but pinned with an LB-issued cookie.
- **Needs:**
  - the SSE adapter to use `credentials: 'include'`;
  - CORS to echo the exact origin with `Allow-Credentials: true` (`origin.go` already echoes allowed
    origins; it has to never use `*`);
  - a check that React Native's `expo/fetch` sends cookies as expected (unverified).
- **Pros:** a stable pin per client; works on managed load balancers that only offer cookie affinity.
- **Cons:** cookie and CORS-credentials surface, and native-platform behavior to verify.

### Option 3: relay SSE sends over Redis (no affinity needed)

- The connection ID gains an instance prefix: `<instanceId>.<random>`. The random part stays the bearer
  secret.
- A `POST /sse/send` that misses the local `Hub` publishes the message to `indri:sse:<instanceId>`. The
  owning instance delivers it and replies on a per-request reply channel. The POST returns once the
  handler finishes, preserving today's "respond after the handler returns" contract, with a timeout
  mapped to `504`.
- Per-connection ordering is still guaranteed: every message for a connection runs on its owning
  instance through the same serialized `Deliver`.
- **Pros:** SSE works behind any load balancer, including plain round-robin.
- **Cons:**
  - An extra Redis round trip on misrouted sends.
  - A request/reply protocol with timeouts to own and test.
  - Instance IDs and liveness become a concern: a dead owner means a timeout, then the client's `404`
    or reconnect path.
  - The earlier plan judged this over-engineering. It's worth it only if a target platform can't do
    affinity.

### WebRTC (no stickiness option needed)

Document the real requirement: each instance advertises its own reachable UDP address. Add a
deployment note and a startup warning when `webrtc` is enabled with the Redis backend and no
`WEBRTC_NAT_1TO1_IPS` is set.

## Recommendation

1. **Part 1 now.** It fixes real multi-instance bugs that stickiness can't fix.
2. **SSE: option 1, plus session resume.** It needs the least code, and resume turns a misroute into a
   brief reconnect instead of a failure. Move to option 3 only if a deployment target can't do
   source-IP or cookie affinity.
3. **WebRTC:** a docs and config change only.

## Decisions for Boss

1. **Target deployment:** Kubernetes, Fly.io, a single VM with nginx, a managed PaaS…? That decides
   whether source-IP affinity is even available, and so between option 1/2 and option 3.
2. **Part 1's scope:** also move the delta path into the new generalized broadcast event, or keep deltas
   on their own existing event? Proposed: keep deltas as they are, because they carry keyframe logic
   (`OpKeyframe`) that must run once per instance.
