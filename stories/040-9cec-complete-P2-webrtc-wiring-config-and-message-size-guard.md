---
id: 040-9cec
title: WebRTC wiring, config and message-size guard
status: complete
priority: P2
type: feature
created: "2026-09-12T01:27:30.682Z"
updated: "2026-09-13T17:02:13.301Z"
dependencies: ["039-233f"]
plan: plans/webrtc-transport.md
plan_step: Step 7
depends_on: ["stories/039-233f-pending-P1-webrtc-transport-and-its-signalling-route.md"]
completed_at: "2026-09-13T17:02:13.301Z"
---

# WebRTC wiring, config and message-size guard

## Problem Statement

The transport has to join the Multi aggregate and take its settings from the environment like everything else. The UDP port must be published by docker-compose or ICE fails silently with nothing wrong at the HTTP layer, and payloads above the practical cross-browser DataChannel ceiling must not be truncated without a word.

## Acceptance Criteria

- [x] The transport is added to transport.NewMulti and given the aggregate via SetPeer
- [x] New INDRI_RTC_ variables are read through the existing env package and documented in .env.example
- [x] docker-compose.yml publishes the UDP port
- [x] A payload over 16 KiB is reported and logged rather than silently truncated
- [x] A broadcast delta reaches WebRTC, WebSocket, GraphQL and SSE clients in the same game identically
- [x] VERIFY: go vet ./... && go test -race ./...

## Files

- internal/injector/services.go
- internal/repo/env/env.go
- docker-compose.yml
- .env.example

## Proof

- [x] [completeness] Completeness (Five criteria covered; the aggregate test registers a conn on each of the four transports and asserts BroadcastFilter reaches all of them.)
- [x] [feature-availability] Feature availability (The transport is constructed and aggregated in services.go; go vet and the boot aggregate test confirm the wiring.)
- [x] [robustness] Robustness (An oversized payload is logged and skipped while the next correctly sized message still goes out, proving the drain does not stall.)
- [x] [resilience] Resilience (webrtcTransport.New returns an error and is handled rather than panicked through, so a port already in use fails boot cleanly.)
- [~] [security] Security (No authentication or authorisation added; this story is wiring and configuration only.)
- [x] [defense-in-depth] Defense in depth (An oversized payload is never handed to the sink, and the 16 KiB ceiling is the conservative cross-browser limit rather than the spec's 256 KiB.)
- [~] [input-validation] Input validation (No external input is introduced; offers are still validated by DecodeSignal.)
- [x] [thread-safety] Thread safety (go test -race -count=2 clean across transport, injector and boot.)
- [x] [configurability] Configurability (Four INDRI_RTC_ variables read through the existing env package, documented in .env.example, with defaults matching the transport's own.)

## Work Log

### 2026-09-13T17:02:12.500Z - Wired the WebRTC transport into the Multi aggregate with SetPeer, added the four INDRI_RTC_ variables, and guarded payloads over the 16 KiB cross-browser DataChannel ceiling so they are logged rather than silently dropped. The compose server service is behind a profile: docker compose up -d is documented as MongoDB and Redis only, and adding a server would bind INDRI_LISTEN_PORT twice so the documented go run step would fail.


### 2026-09-13T17:02:12.597Z - Proof completeness set PROVEN: Five criteria covered; the aggregate test registers a conn on each of the four transports and asserts BroadcastFilter reaches all of them.

### 2026-09-13T17:02:12.667Z - Proof feature-availability set PROVEN: The transport is constructed and aggregated in services.go; go vet and the boot aggregate test confirm the wiring.

### 2026-09-13T17:02:12.736Z - Proof robustness set PROVEN: An oversized payload is logged and skipped while the next correctly sized message still goes out, proving the drain does not stall.

### 2026-09-13T17:02:12.813Z - Proof resilience set PROVEN: webrtcTransport.New returns an error and is handled rather than panicked through, so a port already in use fails boot cleanly.

### 2026-09-13T17:02:12.890Z - Proof security set NOT_APPLICABLE: No authentication or authorisation added; this story is wiring and configuration only.

### 2026-09-13T17:02:12.967Z - Proof defense-in-depth set PROVEN: An oversized payload is never handed to the sink, and the 16 KiB ceiling is the conservative cross-browser limit rather than the spec's 256 KiB.

### 2026-09-13T17:02:13.045Z - Proof input-validation set NOT_APPLICABLE: No external input is introduced; offers are still validated by DecodeSignal.

### 2026-09-13T17:02:13.124Z - Proof thread-safety set PROVEN: go test -race -count=2 clean across transport, injector and boot.

### 2026-09-13T17:02:13.206Z - Proof configurability set PROVEN: Four INDRI_RTC_ variables read through the existing env package, documented in .env.example, with defaults matching the transport's own.
