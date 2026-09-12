---
id: 038-b855
title: Peer lifecycle and teardown
status: pending
priority: P1
type: feature
created: "2026-09-12T01:27:30.676Z"
updated: "2026-09-12T01:27:41.261Z"
dependencies: ["037-68ff"]
plan: plans/webrtc-transport.md
plan_step: Step 5
depends_on: ["stories/037-68ff-pending-P1-peer-factory-and-the-non-trickle-ice-answer.md"]
---

# Peer lifecycle and teardown

## Problem Statement

A peer left in the Registry after its connection fails is a leak and keeps receiving broadcasts that go nowhere. pion makes this easy to get wrong: OnConnectionStateChange runs on a fresh goroutine per invocation with no ordering guarantee against the ICE callback, and Disconnected is transient rather than terminal.

## Acceptance Criteria

- [ ] PeerConnectionStateFailed removes the peer from the Registry and closes its conn
- [ ] PeerConnectionStateClosed does the same
- [ ] PeerConnectionStateDisconnected does not tear the peer down, because it recovers within the pion window
- [ ] Teardown runs exactly once even when state callbacks fire concurrently
- [ ] A race test covers concurrent teardown
- [ ] VERIFY: go test -race ./internal/transport/webrtc/

## Files

- internal/transport/webrtc/peer.go
- internal/transport/webrtc/peer_test.go

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

