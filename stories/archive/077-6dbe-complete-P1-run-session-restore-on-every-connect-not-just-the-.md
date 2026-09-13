---
id: 077-6dbe
title: Run session restore on every connect, not just the first
status: complete
priority: P1
type: fix
created: "2026-09-13T23:20:21.356Z"
updated: "2026-09-13T23:37:29.391Z"
dependencies: ["076-c435"]
plan: plans/client-transport-failover.md
plan_step: Step 5
depends_on: ["stories/076-c435-pending-P1-failover-supervisor-that-is-itself-a-transport.md"]
started_at: "2026-09-13T23:35:51.812Z"
completed_at: "2026-09-13T23:37:29.391Z"
---

# Run session restore on every connect, not just the first

## Problem Statement

MessageHandler.onOpen latches on an opened flag and fires its observers once. SocketProvider uses that hook to replay the stored bearer token, which is what restores identity and game membership. A failover lands on a brand new server-side connection with no session, so unless that hook runs again the player is silently logged out mid-game with their game gone - the exact failure the provider comment says it exists to prevent.

## Acceptance Criteria

- [x] An onOpen observer runs immediately when the transport is already connected
- [x] The same observer runs again on a later reconnect
- [x] An observer that throws does not stop the next one from being notified
- [x] The first-load restore path is behaviour-preserving and its existing tests pass unchanged
- [x] VERIFY: cd client && pnpm test

## Files

- client/services/message-handler.ts
- client/services/message-handler.node-test.ts

## Proof

- [x] [completeness] Completeness (4 new tests in message-handler.node-test.ts cover all 4 behavioural criteria; the 5 pre-existing tests pass unchanged; 395/395)
- [x] [feature-availability] Feature availability (onOpen's public signature is unchanged, so SocketProvider needs no edit to benefit)
- [x] [robustness] Robustness (Observers are iterated from a copy so one unsubscribing during notification cannot skip the next; notifyOpen contains a throw on both the immediate and the reconnect path)
- [x] [resilience] Resilience (This IS the resilience fix: without it a failover silently logs the player out mid-game. Test 'an observer registered while already open still runs on a later reconnect' was RED before the change)
- [x] [security] Security (The restore path replays the stored bearer token over the new channel; no token is logged and none is handled in this file)
- [x] [defense-in-depth] Defense in depth (transport.onClose clears the opened latch, so an observer registered between channels is not told it is connected when it is not)
- [~] [input-validation] Input validation (No external input; onOpen takes a callback from application code only)
- [~] [thread-safety] Thread safety (Single-threaded; the reentrancy case （unsubscribe during notification） is handled by iterating a copy)
- [~] [configurability] Configurability (No configuration surface; the change is to a fixed lifecycle contract)

## QA

None — covered by tests

## Work Log

### 2026-09-13T23:37:17.374Z - MessageHandler.onOpen now subscribes every observer and additionally runs it immediately when already connected, instead of firing once and dropping late subscribers on the floor. The transport's onClose clears the opened latch, so 'already connected' stays truthful across a failover. Notification is funnelled through notifyOpen so a throw is contained on both paths - including the late-registration path, where it previously escaped into the caller. 4 new tests (2 were RED before the fix); 395 total pass.


### 2026-09-13T23:37:28.490Z - Proof completeness set PROVEN: 4 new tests in message-handler.node-test.ts cover all 4 behavioural criteria; the 5 pre-existing tests pass unchanged; 395/395

### 2026-09-13T23:37:28.576Z - Proof feature-availability set PROVEN: onOpen's public signature is unchanged, so SocketProvider needs no edit to benefit

### 2026-09-13T23:37:28.668Z - Proof robustness set PROVEN: Observers are iterated from a copy so one unsubscribing during notification cannot skip the next; notifyOpen contains a throw on both the immediate and the reconnect path

### 2026-09-13T23:37:28.754Z - Proof resilience set PROVEN: This IS the resilience fix: without it a failover silently logs the player out mid-game. Test 'an observer registered while already open still runs on a later reconnect' was RED before the change

### 2026-09-13T23:37:28.874Z - Proof security set PROVEN: The restore path replays the stored bearer token over the new channel; no token is logged and none is handled in this file

### 2026-09-13T23:37:28.984Z - Proof defense-in-depth set PROVEN: transport.onClose clears the opened latch, so an observer registered between channels is not told it is connected when it is not

### 2026-09-13T23:37:29.086Z - Proof input-validation set NOT_APPLICABLE: No external input; onOpen takes a callback from application code only

### 2026-09-13T23:37:29.168Z - Proof thread-safety set NOT_APPLICABLE: Single-threaded; the reentrancy case (unsubscribe during notification) is handled by iterating a copy

### 2026-09-13T23:37:29.278Z - Proof configurability set NOT_APPLICABLE: No configuration surface; the change is to a fixed lifecycle contract
