---
id: 077-6dbe
title: Run session restore on every connect, not just the first
status: pending
priority: P1
type: fix
created: "2026-09-13T23:20:21.356Z"
updated: "2026-09-13T23:20:27.147Z"
dependencies: ["076-c435"]
plan: plans/client-transport-failover.md
plan_step: Step 5
depends_on: ["stories/076-c435-pending-P1-failover-supervisor-that-is-itself-a-transport.md"]
---

# Run session restore on every connect, not just the first

## Problem Statement

MessageHandler.onOpen latches on an opened flag and fires its observers once. SocketProvider uses that hook to replay the stored bearer token, which is what restores identity and game membership. A failover lands on a brand new server-side connection with no session, so unless that hook runs again the player is silently logged out mid-game with their game gone - the exact failure the provider comment says it exists to prevent.

## Acceptance Criteria

- [ ] An onOpen observer runs immediately when the transport is already connected
- [ ] The same observer runs again on a later reconnect
- [ ] An observer that throws does not stop the next one from being notified
- [ ] The first-load restore path is behaviour-preserving and its existing tests pass unchanged
- [ ] VERIFY: cd client && pnpm test

## Files

- client/services/message-handler.ts
- client/services/message-handler.node-test.ts

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

