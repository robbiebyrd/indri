---
id: 075-3eb2
title: SSE and REST client transport
status: complete
priority: P2
type: feature
created: "2026-09-13T23:20:21.355Z"
updated: "2026-09-13T23:29:30.149Z"
dependencies: ["074-1449"]
plan: plans/client-transport-failover.md
plan_step: Step 3
depends_on: ["stories/074-1449-pending-P2-derive-every-server-endpoint-in-one-place.md"]
started_at: "2026-09-13T23:28:01.237Z"
completed_at: "2026-09-13T23:29:30.148Z"
---

# SSE and REST client transport

## Problem Statement

The third channel does not exist on the client. Only WebSocketTransport and WebRTCTransport implement ClientTransport, so the failover chain has nothing to fall back to once both fail. This channel is also structurally different from the other two: it is push-only inbound over EventSource with outbound over POST, and EventSource cannot set headers, so the session token has to travel as a query parameter.

## Acceptance Criteria

- [x] Implements ClientTransport so the supervisor can treat it like any other channel
- [x] Inbound arrives over EventSource and outbound goes over POST to the REST action routes
- [x] connect rejects when no session token is available, because the stream cannot authenticate without one
- [x] The stream URL is never logged, since it carries the bearer token
- [x] Tests cover the pure parts only: the action to route mapping and the no-token rejection
- [x] VERIFY: cd client && pnpm run typecheck

## Files

- client/services/sse-transport.ts
- client/services/sse-transport.node-test.ts

## Proof

- [x] [completeness] Completeness (SseRestTransport implements every ClientTransport member; 10 tests in sse-transport.node-test.ts; 375/375 pass)
- [x] [feature-availability] Feature availability (connect rejects （rather than throws） where EventSource is absent, so the supervisor skips the channel on react-native instead of crashing)
- [x] [robustness] Robustness (A non-OK POST warns and still forwards the server's error document; a network failure is caught; an empty body is not emitted)
- [x] [resilience] Resilience (EventSource's built-in reconnection is suppressed by closing the stream on error and emitting onClose, so the supervisor owns the single backoff)
- [x] [security] Security (Stream URL never logged; test asserts no rejection message contains the token; the REST leg puts the token in an Authorization header, not the query string)
- [x] [defense-in-depth] Defense in depth (Token is encodeURIComponent'd; restRequest gates the action against /^[a-zA-Z][a-zA-Z0-9_]*$/, tested against '../rtc/offer' and 'a/b')
- [x] [input-validation] Input validation (restRequest returns undefined for a missing, non-string, empty or path-shaped action; the send path warns and drops instead of POSTing)
- [~] [thread-safety] Thread safety (Single-threaded event loop; settle（） clears its pending field so a connect settles once)
- [x] [configurability] Configurability (Token is injected as a TokenSource function, re-read per connect and per send, so a login mid-session is picked up without reconstructing the transport)

## QA

None — covered by tests

## Work Log

### 2026-09-13T23:29:18.434Z - Added SseRestTransport: inbound over EventSource GET /events?token=, outbound over POST /api/<action> with a Bearer header. POST response bodies are fed into the same onMessage handlers as stream deltas, so MessageHandler cannot tell a reply from a broadcast. restRequest is the pure action-to-route mapping and carries the one field rename REST needs (reconnect's sessionId -> token, routes.go); it rejects a non-route-name action rather than building a URL from unvalidated input. connect rejects without a token and on a platform with no EventSource, so the channel drops out of the candidate list instead of throwing. EventSource's own reconnection is disabled by closing the stream on error, leaving one backoff owner. 375 tests pass.


### 2026-09-13T23:29:29.432Z - Proof completeness set PROVEN: SseRestTransport implements every ClientTransport member; 10 tests in sse-transport.node-test.ts; 375/375 pass

### 2026-09-13T23:29:29.516Z - Proof feature-availability set PROVEN: connect rejects (rather than throws) where EventSource is absent, so the supervisor skips the channel on react-native instead of crashing

### 2026-09-13T23:29:29.592Z - Proof robustness set PROVEN: A non-OK POST warns and still forwards the server's error document; a network failure is caught; an empty body is not emitted

### 2026-09-13T23:29:29.667Z - Proof resilience set PROVEN: EventSource's built-in reconnection is suppressed by closing the stream on error and emitting onClose, so the supervisor owns the single backoff

### 2026-09-13T23:29:29.741Z - Proof security set PROVEN: Stream URL never logged; test asserts no rejection message contains the token; the REST leg puts the token in an Authorization header, not the query string

### 2026-09-13T23:29:29.820Z - Proof defense-in-depth set PROVEN: Token is encodeURIComponent'd; restRequest gates the action against /^[a-zA-Z][a-zA-Z0-9_]*$/, tested against '../rtc/offer' and 'a/b'

### 2026-09-13T23:29:29.903Z - Proof input-validation set PROVEN: restRequest returns undefined for a missing, non-string, empty or path-shaped action; the send path warns and drops instead of POSTing

### 2026-09-13T23:29:29.981Z - Proof thread-safety set NOT_APPLICABLE: Single-threaded event loop; settle() clears its pending field so a connect settles once

### 2026-09-13T23:29:30.060Z - Proof configurability set PROVEN: Token is injected as a TokenSource function, re-read per connect and per send, so a login mid-session is picked up without reconstructing the transport
