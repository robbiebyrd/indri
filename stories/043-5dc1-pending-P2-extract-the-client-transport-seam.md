---
id: 043-5dc1
title: Extract the client transport seam
status: pending
priority: P2
type: refactor
created: "2026-09-12T01:27:30.684Z"
updated: "2026-09-12T01:27:41.795Z"
dependencies: ["042-38ee"]
plan: plans/webrtc-transport.md
plan_step: Step 10
depends_on: ["stories/042-38ee-pending-P1-spike-react-native-webrtc-under-expo-sdk-53-new-ar.md"]
---

# Extract the client transport seam

## Problem Statement

MessageHandler hardcodes WebSocket at three points, so there is nowhere for a second client transport to plug in. Extracting a small interface mirrors the split the Go side already has and leaves the parser chain untouched.

## Acceptance Criteria

- [ ] A ClientTransport interface covers connect, send, onMessage and close
- [ ] MessageHandler drives the interface rather than a WebSocket directly
- [ ] The WebSocket implementation is behaviour-preserving: send before open is still dropped with a warning
- [ ] Inbound messages reach the existing parsers array unchanged
- [ ] game-state-parser.ts is not modified
- [ ] VERIFY: cd client && pnpm run typecheck

## Files

- client/services/transport.ts
- client/services/message-handler.ts
- client/services/transport.node-test.ts

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

