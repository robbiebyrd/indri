---
id: 076-c435
title: Failover supervisor that is itself a transport
status: pending
priority: P1
type: feature
created: "2026-09-13T23:20:21.356Z"
updated: "2026-09-13T23:20:27.068Z"
dependencies: ["073-9601"]
plan: plans/client-transport-failover.md
plan_step: Step 4
depends_on: ["stories/073-9601-pending-P1-extend-the-client-transport-contract-for-supervisi.md"]
---

# Failover supervisor that is itself a transport

## Problem Statement

Nothing selects a channel or reacts when one dies. The supervisor is the whole design: because it implements ClientTransport itself, MessageHandler keeps taking a single transport and does not change, and the rest of the app cannot tell which channel it is on. Transparency becomes structural rather than something every caller has to cooperate with.

## Acceptance Criteria

- [ ] Implements ClientTransport, so MessageHandler needs no change to use it
- [ ] Picks the first candidate that connects, in preference order
- [ ] Falls through to the next candidate when one rejects or times out
- [ ] Switches to the next candidate when the active channel closes
- [ ] Emits one onOpen per successful connect, including reconnects
- [ ] A send issued before any channel is open is queued and flushed on connect
- [ ] A queued send is dropped with a warning when the channel changes, never replayed, because the server has no idempotency keys
- [ ] The queue is bounded and drops oldest, matching the server's own policy rather than inventing a second one
- [ ] close stops all probing and does not resurrect a channel
- [ ] Backoff prevents a server outage turning into a retry loop across three channels
- [ ] The candidate list is a function re-evaluated per cycle, so SSE is skipped while no token exists
- [ ] VERIFY: cd client && pnpm test

## Files

- client/services/failover-transport.ts
- client/services/failover-transport.node-test.ts

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

