---
id: 079-bd95
title: Document the failover contract
status: complete
priority: P3
type: chore
created: "2026-09-13T23:20:21.357Z"
updated: "2026-09-13T23:41:55.200Z"
dependencies: ["078-516a"]
plan: plans/client-transport-failover.md
plan_step: Step 7
depends_on: ["stories/078-516a-pending-P1-wire-failover-into-the-app-and-resync-after-a-swit.md"]
started_at: "2026-09-13T23:40:56.605Z"
completed_at: "2026-09-13T23:41:55.200Z"
---

# Document the failover contract

## Problem Statement

A client may now arrive on any transport and switch mid-session. Without documenting it, the next reader cannot tell that an action in flight during a switch is deliberately dropped rather than replayed, and would reasonably file that as a bug.

## Acceptance Criteria

- [x] The protocol doc states that a client may arrive on any transport and switch mid-session
- [x] The reconnect plus refresh resync contract is written down
- [x] It is explicit that an action in flight during a switch may be lost, and why dropping beats replaying without idempotency keys
- [x] The TODO about handling reconnects is removed, since this work is that
- [x] VERIFY: cd client && pnpm run typecheck

## Files

- docs/PROTOCOL.md
- README.md
- client/services/transport.ts

## Proof

- [x] [completeness] Completeness (All 4 doc criteria covered by the new 'Transport failover' section in docs/PROTOCOL.md plus README cross-link; typecheck clean)
- [x] [feature-availability] Feature availability (Section is linked from the transports table at the top of PROTOCOL.md and from README's transport table, so it is reachable from both entry points)
- [~] [robustness] Robustness (Documentation change; no runtime behaviour)
- [x] [resilience] Resilience (Documents the resync contract a client must follow to survive a switch, which is the resilience-critical part a future reader would otherwise have to reverse-engineer)
- [x] [security] Security (Records that REST+SSE authenticates at the handshake and cannot be a logged-out client's first channel, and that the session is rebound from the bearer token on the new channel)
- [~] [defense-in-depth] Defense in depth (Documentation change; no code path added)
- [~] [input-validation] Input validation (Documentation change; no input handled)
- [~] [thread-safety] Thread safety (Documentation change; no concurrency)
- [~] [configurability] Configurability (Documentation change; no configuration surface)

## QA

None — documentation only

## Work Log

### 2026-09-13T23:41:45.407Z - Added a Transport failover section to docs/PROTOCOL.md: a client may arrive on any transport and switch mid-session; the reconnect-then-refresh resync contract with why each is needed; why an action in flight during a switch is deliberately dropped rather than replayed (no idempotency keys, so a replay means a failed create or a double-applied move); and why REST+SSE cannot be a logged-out client's first channel. Cross-linked from the transports table and from README. The //TODO: Handle reconnects in client/services/transport.ts is gone - it was removed in 073 when that file was rewritten, and this work is what the TODO was asking for.


### 2026-09-13T23:41:54.391Z - Proof completeness set PROVEN: All 4 doc criteria covered by the new 'Transport failover' section in docs/PROTOCOL.md plus README cross-link; typecheck clean

### 2026-09-13T23:41:54.472Z - Proof feature-availability set PROVEN: Section is linked from the transports table at the top of PROTOCOL.md and from README's transport table, so it is reachable from both entry points

### 2026-09-13T23:41:54.556Z - Proof robustness set NOT_APPLICABLE: Documentation change; no runtime behaviour

### 2026-09-13T23:41:54.643Z - Proof resilience set PROVEN: Documents the resync contract a client must follow to survive a switch, which is the resilience-critical part a future reader would otherwise have to reverse-engineer

### 2026-09-13T23:41:54.740Z - Proof security set PROVEN: Records that REST+SSE authenticates at the handshake and cannot be a logged-out client's first channel, and that the session is rebound from the bearer token on the new channel

### 2026-09-13T23:41:54.827Z - Proof defense-in-depth set NOT_APPLICABLE: Documentation change; no code path added

### 2026-09-13T23:41:54.920Z - Proof input-validation set NOT_APPLICABLE: Documentation change; no input handled

### 2026-09-13T23:41:55.007Z - Proof thread-safety set NOT_APPLICABLE: Documentation change; no concurrency

### 2026-09-13T23:41:55.099Z - Proof configurability set NOT_APPLICABLE: Documentation change; no configuration surface
