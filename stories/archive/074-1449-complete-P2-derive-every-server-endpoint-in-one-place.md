---
id: 074-1449
title: Derive every server endpoint in one place
status: complete
priority: P2
type: refactor
created: "2026-09-13T23:20:21.354Z"
updated: "2026-09-13T23:27:22.704Z"
dependencies: ["073-9601"]
plan: plans/client-transport-failover.md
plan_step: Step 2
depends_on: ["stories/073-9601-pending-P1-extend-the-client-transport-contract-for-supervisi.md"]
started_at: "2026-09-13T23:25:44.210Z"
completed_at: "2026-09-13T23:27:22.704Z"
---

# Derive every server endpoint in one place

## Problem Statement

EXPO_PUBLIC_API_URL is a ws:// URL because WebSocket was once the only transport. Every other channel hangs off the same origin, and webrtc-signal.ts already derives its signalling URL from it. With three channels that derivation needs one home rather than a copy per transport, or they will disagree about the scheme or the path.

## Acceptance Criteria

- [x] A ws:// base yields the WebSocket, WebRTC offer, SSE stream and REST action URLs
- [x] A wss:// base maps to https:// rather than http://
- [x] A base without the /ws suffix still produces correct endpoints
- [x] webrtc-signal.ts consumes the shared helper instead of deriving its own
- [x] VERIFY: cd client && pnpm test

## Files

- client/services/endpoints.ts
- client/services/endpoints.node-test.ts
- client/services/webrtc-signal.ts

## Proof

- [x] [completeness] Completeness (endpoints.ts covers all four routes; 6 tests in endpoints.node-test.ts; signalUrl deleted)
- [x] [feature-availability] Feature availability (endpoints（） exported and already consumed by webrtc-transport.ts:156)
- [x] [robustness] Robustness (test: query string and fragment on the base do not leak into derived routes; base path does not relocate routes)
- [x] [resilience] Resilience (A malformed base throws a named config error instead of producing a wrong URL that would look like an unavailable transport)
- [x] [security] Security (test: wss:// maps to https://, never http:// — a silent downgrade would put the session token on cleartext)
- [x] [defense-in-depth] Defense in depth (Route constants carry the Go file they mirror, so server drift is traceable from one place)
- [x] [input-validation] Input validation (new URL（） parse failure is caught and rethrown as a config error; tests cover empty string and 'not a url')
- [~] [thread-safety] Thread safety (Pure function over a string; no shared mutable state)
- [x] [configurability] Configurability (All four endpoints derive from the single EXPO_PUBLIC_API_URL, so no new environment variables can fall out of step)

## QA

None — covered by tests

## Work Log

### 2026-09-13T23:27:12.838Z - Added client/services/endpoints.ts as the single derivation of the WebSocket, WebRTC offer, SSE and REST routes from EXPO_PUBLIC_API_URL. Deleted signalUrl from webrtc-signal.ts; webrtc-transport.ts now calls endpoints(url).rtcOffer. An empty or malformed base throws a config error naming EXPO_PUBLIC_API_URL rather than silently resolving against the page origin. 365 tests pass.


### 2026-09-13T23:27:21.976Z - Proof completeness set PROVEN: endpoints.ts covers all four routes; 6 tests in endpoints.node-test.ts; signalUrl deleted

### 2026-09-13T23:27:22.051Z - Proof feature-availability set PROVEN: endpoints() exported and already consumed by webrtc-transport.ts:156

### 2026-09-13T23:27:22.126Z - Proof robustness set PROVEN: test: query string and fragment on the base do not leak into derived routes; base path does not relocate routes

### 2026-09-13T23:27:22.201Z - Proof resilience set PROVEN: A malformed base throws a named config error instead of producing a wrong URL that would look like an unavailable transport

### 2026-09-13T23:27:22.280Z - Proof security set PROVEN: test: wss:// maps to https://, never http:// — a silent downgrade would put the session token on cleartext

### 2026-09-13T23:27:22.362Z - Proof defense-in-depth set PROVEN: Route constants carry the Go file they mirror, so server drift is traceable from one place

### 2026-09-13T23:27:22.443Z - Proof input-validation set PROVEN: new URL() parse failure is caught and rethrown as a config error; tests cover empty string and 'not a url'

### 2026-09-13T23:27:22.526Z - Proof thread-safety set NOT_APPLICABLE: Pure function over a string; no shared mutable state

### 2026-09-13T23:27:22.612Z - Proof configurability set PROVEN: All four endpoints derive from the single EXPO_PUBLIC_API_URL, so no new environment variables can fall out of step
