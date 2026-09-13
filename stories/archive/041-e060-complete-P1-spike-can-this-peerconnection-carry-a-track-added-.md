---
id: 041-e060
title: "SPIKE: can this PeerConnection carry a track added after connect"
status: complete
priority: P1
type: task
created: "2026-09-12T01:27:30.683Z"
updated: "2026-09-13T17:22:45.508Z"
dependencies: ["039-233f"]
plan: plans/webrtc-transport.md
plan_step: Step 8
depends_on: ["stories/039-233f-pending-P1-webrtc-transport-and-its-signalling-route.md"]
completed_at: "2026-09-13T17:22:45.508Z"
---

# SPIKE: can this PeerConnection carry a track added after connect

## Problem Statement

Plan blocker P1 Q1. pion ships a renegotiation example, but issues 1073 and 2774 report OnTrack failing to fire when a data-only PeerConnection is upgraded to carry media. Whether phase-two video reuses this connection or needs a second one depends entirely on the answer, and finding out during the video work would be far more expensive than finding out now.

## Acceptance Criteria

- [x] After the DataChannel is open, the server calls AddTrack and drives offer and answer over the signal channel
- [x] The in-process pion client either fires OnTrack or the failure is recorded as the finding
- [x] A second AddTrack issued while the first negotiation is in flight is serialised rather than interleaved
- [x] The yes or no result is written back into the plan under Open Questions
- [x] No media feature code ships: no capture, no routing, no SFU
- [x] If OnTrack does not fire, the spike stops and reports rather than attempting a fix

## Files

- internal/transport/webrtc/renegotiate.go
- internal/transport/webrtc/renegotiation_test.go

## Proof

- [x] [completeness] Completeness (Both the renegotiation path and the serialisation guard have tests; the whole package is green under -race -count=3.)
- [~] [feature-availability] Feature availability (A spike. It answers a question and ships no client-facing feature.)
- [x] [robustness] Robustness (The negative path is a hard failure: the test calls t.Fatal naming issues 1073 and 2774 if OnTrack never fires, so a no would have failed loudly rather than passing vacuously.)
- [x] [resilience] Resilience (Every wait is bounded; a renegotiation that never completes fails the test instead of hanging it.)
- [~] [security] Security (No authentication or authorisation surface; the signal channel still never reaches the action router.)
- [x] [defense-in-depth] Defense in depth (Renegotiation is serialised per peer, guarding the bundled m= section failure in issue 1169, with concurrent AddTrack calls proven to serialise.)
- [x] [input-validation] Input validation (Inbound signal messages are validated by DecodeSignal before reaching pion, reusing story 035's decoder.)
- [x] [thread-safety] Thread safety (Serialisation asserted by peak concurrent offer processing of exactly 1, not by timing; -race -count=3 clean.)
- [~] [configurability] Configurability (No configuration introduced by this spike.)

## Work Log

### 2026-09-13T17:22:44.700Z - SPIKE ANSWERED: YES. A DataChannel-only PeerConnection does carry a track added after connect, on pion v4.2.20 with the API this repo builds. OnTrack fired on all 10 repeated runs under -race; issues 1073 and 2774 did not reproduce. Phase-two video may reuse this connection, so no second PeerConnection is forced. Renegotiation is serialised per peer against issue 1169. No media feature code ships; the track scaffolding lives only in the test.


### 2026-09-13T17:22:44.784Z - Proof completeness set PROVEN: Both the renegotiation path and the serialisation guard have tests; the whole package is green under -race -count=3.

### 2026-09-13T17:22:44.863Z - Proof feature-availability set NOT_APPLICABLE: A spike. It answers a question and ships no client-facing feature.

### 2026-09-13T17:22:44.938Z - Proof robustness set PROVEN: The negative path is a hard failure: the test calls t.Fatal naming issues 1073 and 2774 if OnTrack never fires, so a no would have failed loudly rather than passing vacuously.

### 2026-09-13T17:22:45.015Z - Proof resilience set PROVEN: Every wait is bounded; a renegotiation that never completes fails the test instead of hanging it.

### 2026-09-13T17:22:45.095Z - Proof security set NOT_APPLICABLE: No authentication or authorisation surface; the signal channel still never reaches the action router.

### 2026-09-13T17:22:45.176Z - Proof defense-in-depth set PROVEN: Renegotiation is serialised per peer, guarding the bundled m= section failure in issue 1169, with concurrent AddTrack calls proven to serialise.

### 2026-09-13T17:22:45.258Z - Proof input-validation set PROVEN: Inbound signal messages are validated by DecodeSignal before reaching pion, reusing story 035's decoder.

### 2026-09-13T17:22:45.341Z - Proof thread-safety set PROVEN: Serialisation asserted by peak concurrent offer processing of exactly 1, not by timing; -race -count=3 clean.

### 2026-09-13T17:22:45.423Z - Proof configurability set NOT_APPLICABLE: No configuration introduced by this spike.
