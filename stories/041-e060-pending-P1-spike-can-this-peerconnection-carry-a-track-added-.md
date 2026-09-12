---
id: 041-e060
title: "SPIKE: can this PeerConnection carry a track added after connect"
status: pending
priority: P1
type: task
created: "2026-09-12T01:27:30.683Z"
updated: "2026-09-12T01:27:41.633Z"
dependencies: ["039-233f"]
plan: plans/webrtc-transport.md
plan_step: Step 8
depends_on: ["stories/039-233f-pending-P1-webrtc-transport-and-its-signalling-route.md"]
---

# SPIKE: can this PeerConnection carry a track added after connect

## Problem Statement

Plan blocker P1 Q1. pion ships a renegotiation example, but issues 1073 and 2774 report OnTrack failing to fire when a data-only PeerConnection is upgraded to carry media. Whether phase-two video reuses this connection or needs a second one depends entirely on the answer, and finding out during the video work would be far more expensive than finding out now.

## Acceptance Criteria

- [ ] After the DataChannel is open, the server calls AddTrack and drives offer and answer over the signal channel
- [ ] The in-process pion client either fires OnTrack or the failure is recorded as the finding
- [ ] A second AddTrack issued while the first negotiation is in flight is serialised rather than interleaved
- [ ] The yes or no result is written back into the plan under Open Questions
- [ ] No media feature code ships: no capture, no routing, no SFU
- [ ] If OnTrack does not fire, the spike stops and reports rather than attempting a fix

## Files

- internal/transport/webrtc/renegotiate.go
- internal/transport/webrtc/renegotiation_test.go

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

