---
id: 044-07ae
title: WebRTC client transport
status: complete
priority: P2
type: feature
created: "2026-09-12T01:27:30.685Z"
updated: "2026-09-13T17:46:40.690Z"
dependencies: ["043-5dc1"]
plan: plans/webrtc-transport.md
plan_step: Step 11
depends_on: ["stories/043-5dc1-pending-P2-extract-the-client-transport-seam.md"]
completed_at: "2026-09-13T17:46:40.690Z"
---

# WebRTC client transport

## Problem Statement

The client needs to gather ICE fully, post one offer and apply one answer, then carry game traffic on a reliable ordered DataChannel matching the delta contract. One code path has to serve web and native, which the web shim provides, and an undeclared dependency will pass locally while failing a clean CI install.

## Acceptance Criteria

- [x] react-native-webrtc, the config plugin and the web shim are declared dependencies
- [x] The client gathers ICE fully before posting its offer, matching the non-trickle server
- [x] Separate game and signal DataChannels are created, with game reliable and ordered
- [x] One code path serves web and native through the shim
- [x] Binary and text payload handling is normalised so web and native produce the same JS type
- [x] VERIFY: cd client && pnpm run typecheck && pnpm test

## Files

- client/services/webrtc-transport.ts
- client/services/webrtc-transport.node-test.ts
- client/package.json

## Proof

- [x] [completeness] Completeness (Five criteria each with a test over the pure logic; 335 client tests pass against a clean frozen-lockfile install.)
- [x] [feature-availability] Feature availability (Implements the existing ClientTransport interface, so MessageHandler can take it with no change.)
- [x] [robustness] Robustness (Signal parsing is non-throwing in the repo's defensive style; the ICE gathering wait is bounded and covered for immediate, delayed, pending and timeout cases.)
- [x] [resilience] Resilience (A bounded gathering wait means a stalled negotiation fails instead of hanging silently with nothing in the logs.)
- [~] [security] Security (Client-side transport; authentication is the server's signalling route and the bearer token it already checks.)
- [~] [defense-in-depth] Defense in depth (No new security surface on the client.)
- [x] [input-validation] Input validation (parseSignal rejects malformed answers without throwing, and normaliseMessage is the single place a payload becomes a string.)
- [~] [thread-safety] Thread safety (Single-threaded JavaScript client.)
- [x] [configurability] Configurability (The signalling URL is derived from the existing EXPO_PUBLIC_API_URL, so no new configuration is introduced.)

## Work Log

### 2026-09-13T17:46:39.775Z - WebRTCTransport implements ClientTransport over a DataChannel, gathering ICE fully before posting its offer to match the non-trickle server. Pure logic lives in webrtc-signal.ts because the web shim uses extensionless imports that crash the node test runner on any import from a module touching it - splitting it is what makes the package testable. Verified against a clean frozen-lockfile install: typecheck clean, 335 tests pass.


### 2026-09-13T17:46:39.853Z - Proof completeness set PROVEN: Five criteria each with a test over the pure logic; 335 client tests pass against a clean frozen-lockfile install.

### 2026-09-13T17:46:39.935Z - Proof feature-availability set PROVEN: Implements the existing ClientTransport interface, so MessageHandler can take it with no change.

### 2026-09-13T17:46:40.014Z - Proof robustness set PROVEN: Signal parsing is non-throwing in the repo's defensive style; the ICE gathering wait is bounded and covered for immediate, delayed, pending and timeout cases.

### 2026-09-13T17:46:40.099Z - Proof resilience set PROVEN: A bounded gathering wait means a stalled negotiation fails instead of hanging silently with nothing in the logs.

### 2026-09-13T17:46:40.183Z - Proof security set NOT_APPLICABLE: Client-side transport; authentication is the server's signalling route and the bearer token it already checks.

### 2026-09-13T17:46:40.267Z - Proof defense-in-depth set NOT_APPLICABLE: No new security surface on the client.

### 2026-09-13T17:46:40.355Z - Proof input-validation set PROVEN: parseSignal rejects malformed answers without throwing, and normaliseMessage is the single place a payload becomes a string.

### 2026-09-13T17:46:40.440Z - Proof thread-safety set NOT_APPLICABLE: Single-threaded JavaScript client.

### 2026-09-13T17:46:40.523Z - Proof configurability set PROVEN: The signalling URL is derived from the existing EXPO_PUBLIC_API_URL, so no new configuration is introduced.
