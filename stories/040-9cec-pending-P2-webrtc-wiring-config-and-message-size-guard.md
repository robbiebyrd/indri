---
id: 040-9cec
title: WebRTC wiring, config and message-size guard
status: pending
priority: P2
type: feature
created: "2026-09-12T01:27:30.682Z"
updated: "2026-09-12T01:27:41.560Z"
dependencies: ["039-233f"]
plan: plans/webrtc-transport.md
plan_step: Step 7
depends_on: ["stories/039-233f-pending-P1-webrtc-transport-and-its-signalling-route.md"]
---

# WebRTC wiring, config and message-size guard

## Problem Statement

The transport has to join the Multi aggregate and take its settings from the environment like everything else. The UDP port must be published by docker-compose or ICE fails silently with nothing wrong at the HTTP layer, and payloads above the practical cross-browser DataChannel ceiling must not be truncated without a word.

## Acceptance Criteria

- [ ] The transport is added to transport.NewMulti and given the aggregate via SetPeer
- [ ] New INDRI_RTC_ variables are read through the existing env package and documented in .env.example
- [ ] docker-compose.yml publishes the UDP port
- [ ] A payload over 16 KiB is reported and logged rather than silently truncated
- [ ] A broadcast delta reaches WebRTC, WebSocket, GraphQL and SSE clients in the same game identically
- [ ] VERIFY: go vet ./... && go test -race ./...

## Files

- internal/injector/services.go
- internal/repo/env/env.go
- docker-compose.yml
- .env.example

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

