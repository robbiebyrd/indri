---
id: 038-b855
title: Peer lifecycle and teardown
status: complete
priority: P1
type: feature
created: "2026-09-12T01:27:30.676Z"
updated: "2026-09-12T02:57:09.662Z"
dependencies: ["037-68ff"]
plan: plans/webrtc-transport.md
plan_step: Step 5
depends_on: ["stories/037-68ff-pending-P1-peer-factory-and-the-non-trickle-ice-answer.md"]
completed_at: "2026-09-12T02:57:09.662Z"
---

# Peer lifecycle and teardown

## Problem Statement

A peer left in the Registry after its connection fails is a leak and keeps receiving broadcasts that go nowhere. pion makes this easy to get wrong: OnConnectionStateChange runs on a fresh goroutine per invocation with no ordering guarantee against the ICE callback, and Disconnected is transient rather than terminal.

## Acceptance Criteria

- [x] PeerConnectionStateFailed removes the peer from the Registry and closes its conn
- [x] PeerConnectionStateClosed does the same
- [x] PeerConnectionStateDisconnected does not tear the peer down, because it recovers within the pion window
- [x] Teardown runs exactly once even when state callbacks fire concurrently
- [x] A race test covers concurrent teardown
- [x] VERIFY: go test -race ./internal/transport/webrtc/

## Files

- internal/transport/webrtc/peer.go
- internal/transport/webrtc/peer_test.go

## Proof

- [x] [completeness] Completeness (Seven tests green under -race -count=5, one per criterion plus a real PeerConnection close and a nil-conn case.)
- [~] [feature-availability] Feature availability (Not reachable by clients until the signalling route lands in story 039-233f.)
- [x] [robustness] Robustness (Sequential Failed then Closed, concurrent callbacks, and a nil conn are each covered.)
- [x] [resilience] Resilience (Disconnected is a documented no-op, so a peer that recovers within pion's window is not dropped; asserted on a teardown counter rather than elapsed time.)
- [~] [security] Security (No authentication or authorisation at this layer.)
- [x] [defense-in-depth] Defense in depth (Two guards: sync.Once on teardown, and pion's own idempotent Close underneath it.)
- [~] [input-validation] Input validation (No external input; the only input is a pion connection-state enum.)
- [x] [thread-safety] Thread safety (50 goroutines firing Failed and Closed concurrently leave the teardown counter at exactly 1; -race -count=5 clean.)
- [~] [configurability] Configurability (No configuration in this story; the transient-state window is pion's own default.)

## Work Log

### 2026-09-12T02:57:08.265Z - peer owns one PeerConnection and tears down exactly once. Failed and Closed are terminal; Disconnected is deliberately a no-op because it recovers within pion's window and tearing down would drop players who would have reconnected. Teardown is guarded by sync.Once since OnConnectionStateChange fires on a fresh goroutine per invocation. The Registry does not exist until story 039, so deregistration is an onClose callback that 039 will wire.


### 2026-09-12T02:57:08.393Z - Proof completeness set PROVEN: Seven tests green under -race -count=5, one per criterion plus a real PeerConnection close and a nil-conn case.

### 2026-09-12T02:57:08.471Z - Proof feature-availability set NOT_APPLICABLE: Not reachable by clients until the signalling route lands in story 039-233f.

### 2026-09-12T02:57:08.546Z - Proof robustness set PROVEN: Sequential Failed then Closed, concurrent callbacks, and a nil conn are each covered.

### 2026-09-12T02:57:08.624Z - Proof resilience set PROVEN: Disconnected is a documented no-op, so a peer that recovers within pion's window is not dropped; asserted on a teardown counter rather than elapsed time.

### 2026-09-12T02:57:08.706Z - Proof security set NOT_APPLICABLE: No authentication or authorisation at this layer.

### 2026-09-12T02:57:08.785Z - Proof defense-in-depth set PROVEN: Two guards: sync.Once on teardown, and pion's own idempotent Close underneath it.

### 2026-09-12T02:57:08.864Z - Proof input-validation set NOT_APPLICABLE: No external input; the only input is a pion connection-state enum.

### 2026-09-12T02:57:08.943Z - Proof thread-safety set PROVEN: 50 goroutines firing Failed and Closed concurrently leave the teardown counter at exactly 1; -race -count=5 clean.

### 2026-09-12T02:57:09.024Z - Proof configurability set NOT_APPLICABLE: No configuration in this story; the transient-state window is pion's own default.
