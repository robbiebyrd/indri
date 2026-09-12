---
id: 037-68ff
title: Peer factory and the non-trickle ICE answer
status: complete
priority: P1
type: feature
created: "2026-09-12T01:27:30.675Z"
updated: "2026-09-12T02:48:45.921Z"
dependencies: ["035-f2e8"]
plan: plans/webrtc-transport.md
plan_step: Step 4
depends_on: ["stories/035-f2e8-pending-P2-webrtc-signal-envelope-decoding.md"]
completed_at: "2026-09-12T02:48:45.921Z"
---

# Peer factory and the non-trickle ICE answer

## Problem Statement

The server needs one pion API built once, carrying an explicit MediaEngine and a single-port UDP mux, and it must answer an offer with a complete SDP. Codec registration is the one thing that cannot be added after a PeerConnection exists, so registering now is what keeps audio and video open later at no cost today.

## Acceptance Criteria

- [x] A single webrtc.API is built once per transport with RegisterDefaultCodecs called exactly once
- [x] All peers share one UDP port via a mux created before any PeerConnection uses it
- [x] SetNAT1To1IPs is applied when configured
- [x] An offer produces an answer whose SDP already contains candidates
- [x] The gathering wait selects against a context and is never a bare channel receive
- [x] A PeerConnection closed mid-gather returns an error rather than hanging
- [x] An in-process test drives both peers with pion and exchanges messages both ways
- [x] VERIFY: go test -race ./internal/transport/webrtc/

## Files

- internal/transport/webrtc/peer.go
- internal/transport/webrtc/peer_test.go

## Proof

- [x] [completeness] Completeness (Seven tests, one per criterion, green under -race -count=3.)
- [~] [feature-availability] Feature availability (Not reachable by clients until the signalling route lands in story 039-233f.)
- [x] [robustness] Robustness (A PeerConnection closed mid-gather returns an error rather than a truncated SDP or a hang; the test forces a real gathering window with an unreachable STUN address.)
- [x] [resilience] Resilience (The gather wait selects against a context and additionally checks SignalingStateClosed, since pion fires the gather promise on close as well.)
- [~] [security] Security (No authentication or authorisation at this layer; the signalling route owns that in story 039-233f.)
- [x] [defense-in-depth] Defense in depth (Two independent guards on the gather wait: context cancellation and the post-select signalling-state check.)
- [~] [input-validation] Input validation (Offers arrive already validated by DecodeSignal from story 035-f2e8.)
- [x] [thread-safety] Thread safety (One API and one mux are shared across peers by design; go test -race -count=3 clean.)
- [x] [configurability] Configurability (UDPPort and NAT1To1IPs are Config fields, each covered by a test; the port is not hardcoded.)

## Work Log

### 2026-09-12T02:48:30.466Z - Peer factory built: one pion API per transport with codecs registered at construction, one shared UDP mux, and a non-trickle answer. Two corrections to the plan: GatheringCompletePromise also fires on PeerConnection close, so the context select alone would return a truncated SDP with a nil error - guarded with SignalingStateClosed. And the shared-port test needed a concrete reserved port, since the mux binds one socket per interface and port 0 gives each a different one.


### 2026-09-12T02:48:39.974Z - Proof completeness set PROVEN: Seven tests, one per criterion, green under -race -count=3.

### 2026-09-12T02:48:40.058Z - Proof feature-availability set NOT_APPLICABLE: Not reachable by clients until the signalling route lands in story 039-233f.

### 2026-09-12T02:48:40.170Z - Proof robustness set PROVEN: A PeerConnection closed mid-gather returns an error rather than a truncated SDP or a hang; the test forces a real gathering window with an unreachable STUN address.

### 2026-09-12T02:48:40.266Z - Proof resilience set PROVEN: The gather wait selects against a context and additionally checks SignalingStateClosed, since pion fires the gather promise on close as well.

### 2026-09-12T02:48:40.386Z - Proof security set NOT_APPLICABLE: No authentication or authorisation at this layer; the signalling route owns that in story 039-233f.

### 2026-09-12T02:48:40.495Z - Proof defense-in-depth set PROVEN: Two independent guards on the gather wait: context cancellation and the post-select signalling-state check.

### 2026-09-12T02:48:40.593Z - Proof input-validation set NOT_APPLICABLE: Offers arrive already validated by DecodeSignal from story 035-f2e8.

### 2026-09-12T02:48:40.685Z - Proof thread-safety set PROVEN: One API and one mux are shared across peers by design; go test -race -count=3 clean.

### 2026-09-12T02:48:40.784Z - Proof configurability set PROVEN: UDPPort and NAT1To1IPs are Config fields, each covered by a test; the port is not hardcoded.
