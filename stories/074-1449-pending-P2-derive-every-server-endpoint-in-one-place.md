---
id: 074-1449
title: Derive every server endpoint in one place
status: pending
priority: P2
type: refactor
created: "2026-09-13T23:20:21.354Z"
updated: "2026-09-13T23:20:26.917Z"
dependencies: ["073-9601"]
plan: plans/client-transport-failover.md
plan_step: Step 2
depends_on: ["stories/073-9601-pending-P1-extend-the-client-transport-contract-for-supervisi.md"]
---

# Derive every server endpoint in one place

## Problem Statement

EXPO_PUBLIC_API_URL is a ws:// URL because WebSocket was once the only transport. Every other channel hangs off the same origin, and webrtc-signal.ts already derives its signalling URL from it. With three channels that derivation needs one home rather than a copy per transport, or they will disagree about the scheme or the path.

## Acceptance Criteria

- [ ] A ws:// base yields the WebSocket, WebRTC offer, SSE stream and REST action URLs
- [ ] A wss:// base maps to https:// rather than http://
- [ ] A base without the /ws suffix still produces correct endpoints
- [ ] webrtc-signal.ts consumes the shared helper instead of deriving its own
- [ ] VERIFY: cd client && pnpm test

## Files

- client/services/endpoints.ts
- client/services/endpoints.node-test.ts
- client/services/webrtc-signal.ts

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

