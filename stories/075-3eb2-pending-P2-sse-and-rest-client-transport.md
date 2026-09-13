---
id: 075-3eb2
title: SSE and REST client transport
status: pending
priority: P2
type: feature
created: "2026-09-13T23:20:21.355Z"
updated: "2026-09-13T23:20:26.993Z"
dependencies: ["074-1449"]
plan: plans/client-transport-failover.md
plan_step: Step 3
depends_on: ["stories/074-1449-pending-P2-derive-every-server-endpoint-in-one-place.md"]
---

# SSE and REST client transport

## Problem Statement

The third channel does not exist on the client. Only WebSocketTransport and WebRTCTransport implement ClientTransport, so the failover chain has nothing to fall back to once both fail. This channel is also structurally different from the other two: it is push-only inbound over EventSource with outbound over POST, and EventSource cannot set headers, so the session token has to travel as a query parameter.

## Acceptance Criteria

- [ ] Implements ClientTransport so the supervisor can treat it like any other channel
- [ ] Inbound arrives over EventSource and outbound goes over POST to the REST action routes
- [ ] connect rejects when no session token is available, because the stream cannot authenticate without one
- [ ] The stream URL is never logged, since it carries the bearer token
- [ ] Tests cover the pure parts only: the action to route mapping and the no-token rejection
- [ ] VERIFY: cd client && pnpm run typecheck

## Files

- client/services/sse-transport.ts
- client/services/sse-transport.node-test.ts

## Proof

- [ ] [completeness] Completeness
- [ ] [feature-availability] Feature availability
- [ ] [robustness] Robustness
- [ ] [resilience] Resilience
- [ ] [security] Security
- [ ] [defense-in-depth] Defense in depth
- [ ] [input-validation] Input validation
- [ ] [thread-safety] Thread safety
- [ ] [configurability] Configurability

## Work Log

