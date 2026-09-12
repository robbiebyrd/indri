---
id: 039-233f
title: WebRTC transport and its signalling route
status: ready
priority: P1
type: feature
created: "2026-09-12T01:27:30.677Z"
updated: "2026-09-12T02:58:06.133Z"
dependencies: ["034-e42b", "036-f0c7", "038-b855"]
plan: plans/webrtc-transport.md
plan_step: Step 6
depends_on: ["stories/034-e42b-pending-P2-anonymous-connections-carry-no-session-key.md", "stories/036-f0c7-pending-P2-datachannel-backed-transport-connection.md", "stories/038-b855-pending-P1-peer-lifecycle-and-teardown.md"]
---

# WebRTC transport and its signalling route

## Problem Statement

WebRTC needs to stand alone as a transport, so signalling lives in the transport rather than the action registry. That allows an anonymous peer whose session binds when login arrives over the DataChannel, but it also means unauthenticated callers can allocate PeerConnections in a loop, so a TTL and a cap are part of the feature rather than polish.

## Acceptance Criteria

- [ ] POST to the signalling route with a pion-generated offer returns an answer and registers a conn
- [ ] A request carrying a valid bearer token binds SessionIDKey at handshake so BroadcastFilter reaches it
- [ ] A request with no token yields an anonymous conn carrying no session key
- [ ] A login sent over the DataChannel binds the session through the existing dispatch path, with no new action code
- [ ] A peer that never reaches connected is closed and dropped after the pending TTL
- [ ] Exceeding the global peer cap returns 503 rather than allocating
- [ ] Disconnect closes the peer, reaching it across transports through the aggregate
- [ ] The game and signal DataChannels are routed by label
- [ ] Messages on the signal channel never reach the action router
- [ ] VERIFY: go test -race ./internal/transport/webrtc/

## Files

- internal/transport/webrtc/webrtc.go
- internal/transport/webrtc/webrtc_test.go

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

