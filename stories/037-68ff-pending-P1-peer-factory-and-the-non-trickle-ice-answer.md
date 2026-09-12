---
id: 037-68ff
title: Peer factory and the non-trickle ICE answer
status: pending
priority: P1
type: feature
created: "2026-09-12T01:27:30.675Z"
updated: "2026-09-12T01:27:41.189Z"
dependencies: ["035-f2e8"]
plan: plans/webrtc-transport.md
plan_step: Step 4
depends_on: ["stories/035-f2e8-pending-P2-webrtc-signal-envelope-decoding.md"]
---

# Peer factory and the non-trickle ICE answer

## Problem Statement

The server needs one pion API built once, carrying an explicit MediaEngine and a single-port UDP mux, and it must answer an offer with a complete SDP. Codec registration is the one thing that cannot be added after a PeerConnection exists, so registering now is what keeps audio and video open later at no cost today.

## Acceptance Criteria

- [ ] A single webrtc.API is built once per transport with RegisterDefaultCodecs called exactly once
- [ ] All peers share one UDP port via a mux created before any PeerConnection uses it
- [ ] SetNAT1To1IPs is applied when configured
- [ ] An offer produces an answer whose SDP already contains candidates
- [ ] The gathering wait selects against a context and is never a bare channel receive
- [ ] A PeerConnection closed mid-gather returns an error rather than hanging
- [ ] An in-process test drives both peers with pion and exchanges messages both ways
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

