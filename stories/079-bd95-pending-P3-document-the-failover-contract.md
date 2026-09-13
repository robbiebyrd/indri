---
id: 079-bd95
title: Document the failover contract
status: pending
priority: P3
type: chore
created: "2026-09-13T23:20:21.357Z"
updated: "2026-09-13T23:20:27.396Z"
dependencies: ["078-516a"]
plan: plans/client-transport-failover.md
plan_step: Step 7
depends_on: ["stories/078-516a-pending-P1-wire-failover-into-the-app-and-resync-after-a-swit.md"]
---

# Document the failover contract

## Problem Statement

A client may now arrive on any transport and switch mid-session. Without documenting it, the next reader cannot tell that an action in flight during a switch is deliberately dropped rather than replayed, and would reasonably file that as a bug.

## Acceptance Criteria

- [ ] The protocol doc states that a client may arrive on any transport and switch mid-session
- [ ] The reconnect plus refresh resync contract is written down
- [ ] It is explicit that an action in flight during a switch may be lost, and why dropping beats replaying without idempotency keys
- [ ] The TODO about handling reconnects is removed, since this work is that
- [ ] VERIFY: cd client && pnpm run typecheck

## Files

- docs/PROTOCOL.md
- README.md
- client/services/transport.ts

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

