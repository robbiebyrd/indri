---
id: 076-c435
title: Failover supervisor that is itself a transport
status: complete
priority: P1
type: feature
created: "2026-09-13T23:20:21.356Z"
updated: "2026-09-13T23:35:26.441Z"
dependencies: ["073-9601"]
plan: plans/client-transport-failover.md
plan_step: Step 4
depends_on: ["stories/073-9601-pending-P1-extend-the-client-transport-contract-for-supervisi.md"]
started_at: "2026-09-13T23:30:40.557Z"
completed_at: "2026-09-13T23:35:26.441Z"
---

# Failover supervisor that is itself a transport

## Problem Statement

Nothing selects a channel or reacts when one dies. The supervisor is the whole design: because it implements ClientTransport itself, MessageHandler keeps taking a single transport and does not change, and the rest of the app cannot tell which channel it is on. Transparency becomes structural rather than something every caller has to cooperate with.

## Acceptance Criteria

- [x] Implements ClientTransport, so MessageHandler needs no change to use it
- [x] Picks the first candidate that connects, in preference order
- [x] Falls through to the next candidate when one rejects or times out
- [x] Switches to the next candidate when the active channel closes
- [x] Emits one onOpen per successful connect, including reconnects
- [x] A send issued before any channel is open is queued and flushed on connect
- [x] A queued send is dropped with a warning when the channel changes, never replayed, because the server has no idempotency keys
- [x] The queue is bounded and drops oldest, matching the server's own policy rather than inventing a second one
- [x] close stops all probing and does not resurrect a channel
- [x] Backoff prevents a server outage turning into a retry loop across three channels
- [x] The candidate list is a function re-evaluated per cycle, so SSE is skipped while no token exists
- [x] VERIFY: cd client && pnpm test

## Files

- client/services/failover-transport.ts
- client/services/failover-transport.node-test.ts

## Proof

- [x] [completeness] Completeness (All 11 behavioural criteria have a dedicated test in failover-transport.node-test.ts; 16/16 pass, 391 total)
- [x] [feature-availability] Feature availability (Typed as ClientTransport in a test, so substitutability is compiler-checked, not asserted in prose)
- [x] [robustness] Robustness (Tests cover a hanging candidate, an empty candidate list, a stale channel still emitting frames, and close（） during an in-flight probe)
- [x] [resilience] Resilience (Fixed a re-entrancy race where a channel dying while the loop unwound swallowed the restart; single-loop plus lost-latch removes it. Backoff test asserts no retry inside the window)
- [x] [security] Security (No credentials handled; the supervisor never logs a URL, and a candidate is closed before being abandoned so no half-open authenticated channel is left running)
- [x] [defense-in-depth] Defense in depth (lost（） ignores a close from a non-active channel; probe re-checks closed after every await; connectWithTimeout closes what it abandons)
- [~] [input-validation] Input validation (No external input; the supervisor moves opaque messages and never inspects them)
- [x] [thread-safety] Thread safety (Single-threaded, but the async re-entrancy equivalent was the real defect found and fixed; the lost-latch handles a signal arriving before anyone waits)
- [x] [configurability] Configurability (attemptTimeoutMs, backoffMs and queueLimit are constructor options; the candidate list is a caller-supplied function re-evaluated per cycle)

## QA

None — covered by tests

## Work Log

### 2026-09-13T23:35:13.548Z - Added FailoverTransport, which implements ClientTransport itself so MessageHandler is unchanged. One long-lived selection loop: probe candidates in preference order, sit on the adopted channel until it dies, then resume AFTER it; a cycle with no winner backs off. Found and fixed a real re-entrancy race on the way: the first shape (adopt, return, restart on close) swallowed the restart when a channel died while the loop was still unwinding, stranding the supervisor with no channel and nothing probing. The single loop plus a lost-latch removes the race by construction. Queue is bounded drop-oldest and is discarded whenever a channel is abandoned, so nothing is replayed across a switch. onOpen observers run before the queue flush, which is what lets reconnect precede any application message. 16 new tests; 391 total pass; typecheck and lint clean.


### 2026-09-13T23:35:25.707Z - Proof completeness set PROVEN: All 11 behavioural criteria have a dedicated test in failover-transport.node-test.ts; 16/16 pass, 391 total

### 2026-09-13T23:35:25.781Z - Proof feature-availability set PROVEN: Typed as ClientTransport in a test, so substitutability is compiler-checked, not asserted in prose

### 2026-09-13T23:35:25.857Z - Proof robustness set PROVEN: Tests cover a hanging candidate, an empty candidate list, a stale channel still emitting frames, and close() during an in-flight probe

### 2026-09-13T23:35:25.939Z - Proof resilience set PROVEN: Fixed a re-entrancy race where a channel dying while the loop unwound swallowed the restart; single-loop plus lost-latch removes it. Backoff test asserts no retry inside the window

### 2026-09-13T23:35:26.015Z - Proof security set PROVEN: No credentials handled; the supervisor never logs a URL, and a candidate is closed before being abandoned so no half-open authenticated channel is left running

### 2026-09-13T23:35:26.093Z - Proof defense-in-depth set PROVEN: lost() ignores a close from a non-active channel; probe re-checks closed after every await; connectWithTimeout closes what it abandons

### 2026-09-13T23:35:26.171Z - Proof input-validation set NOT_APPLICABLE: No external input; the supervisor moves opaque messages and never inspects them

### 2026-09-13T23:35:26.254Z - Proof thread-safety set PROVEN: Single-threaded, but the async re-entrancy equivalent was the real defect found and fixed; the lost-latch handles a signal arriving before anyone waits

### 2026-09-13T23:35:26.335Z - Proof configurability set PROVEN: attemptTimeoutMs, backoffMs and queueLimit are constructor options; the candidate list is a caller-supplied function re-evaluated per cycle
