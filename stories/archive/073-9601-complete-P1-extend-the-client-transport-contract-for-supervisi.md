---
id: 073-9601
title: Extend the client transport contract for supervision
status: complete
priority: P1
type: refactor
created: "2026-09-13T23:20:21.335Z"
updated: "2026-09-13T23:25:29.796Z"
dependencies: []
plan: plans/client-transport-failover.md
plan_step: Step 1
started_at: "2026-09-13T23:22:21.006Z"
completed_at: "2026-09-13T23:25:29.796Z"
---

# Extend the client transport contract for supervision

## Problem Statement

ClientTransport cannot support a supervisor today. onMessage and onOpen are setters rather than subscriptions, so only one sink can ever exist and nothing can attach or detach per channel. There is no failure signal at all: onerror and onclose are swallowed inside WebSocketTransport behind a TODO, so nothing can trigger a failover. And connect returns void, so a selection loop has no way to know whether a channel came up.

## Acceptance Criteria

- [x] onMessage and onOpen return working unsubscribe functions
- [x] onClose reports a closed or failed connection with a reason, which is the signal a supervisor fails over on
- [x] connect returns a promise that resolves on open and rejects on failure or timeout
- [x] Each transport carries a name for diagnostics that is never used as a branch target
- [x] WebSocketTransport is behaviour-preserving: send before open is still dropped with the same warning
- [x] MessageHandler still works when handed a WebSocketTransport directly, with its existing tests unchanged
- [x] VERIFY: cd client && pnpm run typecheck

## Files

- client/services/transport.ts
- client/services/transport.node-test.ts

## Proof

- [x] [completeness] Completeness (All 7 criteria implemented in client/services/transport.ts; 11 new tests in transport.node-test.ts, 361/361 pass)
- [x] [feature-availability] Feature availability (connectWithTimeout + onClose exported and covered by tests; WebRTCTransport migrated to the same contract)
- [x] [robustness] Robustness (test: 'a throwing handler does not stop the next one being notified'; Handlers.emit iterates a copy and try/catches each handler)
- [x] [resilience] Resilience (connect rejects on close-before-open; close（） settles a pending connect so no promise leaks; connectWithTimeout closes the abandoned socket （test asserts readyState 3）)
- [x] [security] Security (No credentials touched; onerror no longer console.logs the raw Event, which on some platforms carries the URL)
- [x] [defense-in-depth] Defense in depth (settle（） is idempotent via a cleared pending field; connectWithTimeout guards with its own settled flag, so a late resolve after timeout is ignored)
- [~] [input-validation] Input validation (No external input is parsed here; inbound frame payloads are validated downstream by MessageHandler's parseJsonSafely)
- [~] [thread-safety] Thread safety (JavaScript single-threaded event loop; the one reentrancy hazard （unsubscribe during emit） is handled by iterating a copy)
- [x] [configurability] Configurability (Attempt budget is a connectWithTimeout parameter rather than a hardcoded constant, so the supervisor owns the policy)

## QA

None — covered by tests

## Work Log

### 2026-09-13T23:25:14.595Z - Extended ClientTransport for supervision: name, Promise-returning connect, subscription-shaped onMessage/onOpen, new onClose failover signal. Shared Handlers fan-out class (unsubscribe-during-emit safe, throw-isolating) reused by WebSocketTransport and WebRTCTransport. connectWithTimeout is the single place attempt-bounding policy lives, and closes the abandoned socket. A deliberate close() never fires onClose, so a supervisor cannot fail over on its own teardown. 361 tests pass (was 350), typecheck and lint clean.


### 2026-09-13T23:25:27.361Z - Proof completeness set PROVEN: All 7 criteria implemented in client/services/transport.ts; 11 new tests in transport.node-test.ts, 361/361 pass

### 2026-09-13T23:25:27.434Z - Proof feature-availability set PROVEN: connectWithTimeout + onClose exported and covered by tests; WebRTCTransport migrated to the same contract

### 2026-09-13T23:25:27.510Z - Proof robustness set PROVEN: test: 'a throwing handler does not stop the next one being notified'; Handlers.emit iterates a copy and try/catches each handler

### 2026-09-13T23:25:27.591Z - Proof resilience set PROVEN: connect rejects on close-before-open; close() settles a pending connect so no promise leaks; connectWithTimeout closes the abandoned socket (test asserts readyState 3)

### 2026-09-13T23:25:27.675Z - Proof security set PROVEN: No credentials touched; onerror no longer console.logs the raw Event, which on some platforms carries the URL

### 2026-09-13T23:25:27.759Z - Proof defense-in-depth set PROVEN: settle() is idempotent via a cleared pending field; connectWithTimeout guards with its own settled flag, so a late resolve after timeout is ignored

### 2026-09-13T23:25:27.843Z - Proof input-validation set NOT_APPLICABLE: No external input is parsed here; inbound frame payloads are validated downstream by MessageHandler's parseJsonSafely

### 2026-09-13T23:25:27.925Z - Proof thread-safety set NOT_APPLICABLE: JavaScript single-threaded event loop; the one reentrancy hazard (unsubscribe during emit) is handled by iterating a copy

### 2026-09-13T23:25:28.004Z - Proof configurability set PROVEN: Attempt budget is a connectWithTimeout parameter rather than a hardcoded constant, so the supervisor owns the policy
