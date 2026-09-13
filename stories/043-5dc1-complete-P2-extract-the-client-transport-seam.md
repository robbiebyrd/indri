---
id: 043-5dc1
title: Extract the client transport seam
status: complete
priority: P2
type: refactor
created: "2026-09-12T01:27:30.684Z"
updated: "2026-09-13T17:33:30.148Z"
dependencies: ["042-38ee"]
plan: plans/webrtc-transport.md
plan_step: Step 10
depends_on: ["stories/042-38ee-pending-P1-spike-react-native-webrtc-under-expo-sdk-53-new-ar.md"]
completed_at: "2026-09-13T17:33:30.148Z"
---

# Extract the client transport seam

## Problem Statement

MessageHandler hardcodes WebSocket at three points, so there is nowhere for a second client transport to plug in. Extracting a small interface mirrors the split the Go side already has and leaves the parser chain untouched.

## Acceptance Criteria

- [x] A ClientTransport interface covers connect, send, onMessage and close
- [x] MessageHandler drives the interface rather than a WebSocket directly
- [x] The WebSocket implementation is behaviour-preserving: send before open is still dropped with a warning
- [x] Inbound messages reach the existing parsers array unchanged
- [x] game-state-parser.ts is not modified
- [x] VERIFY: cd client && pnpm run typecheck

## Files

- client/services/transport.ts
- client/services/message-handler.ts
- client/services/transport.node-test.ts

## Proof

- [x] [completeness] Completeness (Five criteria covered; 310 client tests pass and typecheck is clean.)
- [x] [feature-availability] Feature availability (Behaviour-preserving: existing four-argument call sites are untouched because the transport parameter defaults to the WebSocket implementation.)
- [x] [robustness] Robustness (Send before open is still dropped with the same warning, and the onmessage teardown survives; both covered by tests.)
- [~] [resilience] Resilience (A refactor with no new failure modes; reconnect handling remains the pre-existing TODO it was.)
- [~] [security] Security (No authentication or authorisation touched.)
- [~] [defense-in-depth] Defense in depth (No security surface in a transport interface extraction.)
- [~] [input-validation] Input validation (No input validation changed; routeIncomingMessage keeps its exact signature.)
- [~] [thread-safety] Thread safety (Single-threaded JavaScript client.)
- [x] [configurability] Configurability (The transport is injectable via the constructor, which is the entire point of the seam.)

## Work Log

### 2026-09-13T17:33:25.434Z - Extracted ClientTransport plus WebSocketTransport; MessageHandler now takes one as an optional sixth parameter defaulting to the WebSocket implementation, so existing call sites are unchanged. Behaviour preserved verbatim including the send-before-open warning and the onmessage teardown. onOpen was added beyond the planned four methods because the existing open-observer API feeds socket-provider session restore and that notion is transport-specific.


### 2026-09-13T17:33:25.514Z - Proof completeness set PROVEN: Five criteria covered; 310 client tests pass and typecheck is clean.

### 2026-09-13T17:33:25.590Z - Proof feature-availability set PROVEN: Behaviour-preserving: existing four-argument call sites are untouched because the transport parameter defaults to the WebSocket implementation.

### 2026-09-13T17:33:25.658Z - Proof robustness set PROVEN: Send before open is still dropped with the same warning, and the onmessage teardown survives; both covered by tests.

### 2026-09-13T17:33:25.735Z - Proof resilience set NOT_APPLICABLE: A refactor with no new failure modes; reconnect handling remains the pre-existing TODO it was.

### 2026-09-13T17:33:25.817Z - Proof security set NOT_APPLICABLE: No authentication or authorisation touched.

### 2026-09-13T17:33:25.897Z - Proof defense-in-depth set NOT_APPLICABLE: No security surface in a transport interface extraction.

### 2026-09-13T17:33:25.977Z - Proof input-validation set NOT_APPLICABLE: No input validation changed; routeIncomingMessage keeps its exact signature.

### 2026-09-13T17:33:26.055Z - Proof thread-safety set NOT_APPLICABLE: Single-threaded JavaScript client.

### 2026-09-13T17:33:26.134Z - Proof configurability set PROVEN: The transport is injectable via the constructor, which is the entire point of the seam.
