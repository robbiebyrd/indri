---
id: 035-f2e8
title: WebRTC signal envelope decoding
status: complete
priority: P2
type: feature
created: "2026-09-12T01:27:30.674Z"
updated: "2026-09-12T01:37:40.565Z"
dependencies: []
plan: plans/webrtc-transport.md
plan_step: Step 2
completed_at: "2026-09-12T01:37:40.565Z"
---

# WebRTC signal envelope decoding

## Problem Statement

SDP arrives from unauthenticated callers and is parsed by pion. The transport needs a defensive decode step so malformed, oversized or wrongly typed payloads are rejected with a clear error before any of it reaches the WebRTC stack.

## Acceptance Criteria

- [x] A valid offer decodes into a Signal carrying type and sdp
- [x] A payload missing sdp is rejected with a distinct error
- [x] A payload with a type other than offer or answer is rejected
- [x] Malformed JSON is rejected rather than panicking
- [x] The request body is capped at 256 KiB before decoding
- [x] No rejected input reaches pion
- [x] VERIFY: go test -race ./internal/transport/webrtc/

## Files

- internal/transport/webrtc/signal.go
- internal/transport/webrtc/signal_test.go

## Proof

- [x] [completeness] Completeness (Nine tests cover the happy path plus every rejection path named in the criteria.)
- [~] [feature-availability] Feature availability (Not reachable by clients until the signalling route lands in story 039-233f.)
- [x] [robustness] Robustness (Malformed JSON, empty body, missing sdp and an unknown type are each tested and return a nil Signal.)
- [~] [resilience] Resilience (Pure function over an io.Reader; no external dependency that can fail.)
- [x] [security] Security (SDP is attacker-controlled and unauthenticated: the body is capped at 256 KiB and fully validated before anything could reach pion.)
- [x] [defense-in-depth] Defense in depth (Three independent layers: size cap, type allowlist, required-field check. Nothing in the file imports pion.)
- [x] [input-validation] Input validation (Input validation is the entire subject of the story; every criterion is a validation case with a test.)
- [~] [thread-safety] Thread safety (Pure function with no shared state.)
- [~] [configurability] Configurability (The 256 KiB cap is deliberately a constant; a tunable cap would be an attack surface.)

## QA

go test -race ./internal/transport/webrtc/ green; no pion dependency added.

## Work Log

### 2026-09-12T01:36:39.083Z - DecodeSignal validates type, sdp and body size before anything reaches pion. First draft used json.Decoder over a LimitReader, which buffers ahead and could let an oversized body through; replaced with read-then-check-then-unmarshal.


### 2026-09-12T01:37:35.555Z - Proof completeness set PROVEN: Nine tests cover the happy path plus every rejection path named in the criteria.

### 2026-09-12T01:37:35.638Z - Proof feature-availability set NOT_APPLICABLE: Not reachable by clients until the signalling route lands in story 039-233f.

### 2026-09-12T01:37:35.720Z - Proof robustness set PROVEN: Malformed JSON, empty body, missing sdp and an unknown type are each tested and return a nil Signal.

### 2026-09-12T01:37:35.804Z - Proof resilience set NOT_APPLICABLE: Pure function over an io.Reader; no external dependency that can fail.

### 2026-09-12T01:37:35.884Z - Proof security set PROVEN: SDP is attacker-controlled and unauthenticated: the body is capped at 256 KiB and fully validated before anything could reach pion.

### 2026-09-12T01:37:35.968Z - Proof defense-in-depth set PROVEN: Three independent layers: size cap, type allowlist, required-field check. Nothing in the file imports pion.

### 2026-09-12T01:37:36.040Z - Proof input-validation set PROVEN: Input validation is the entire subject of the story; every criterion is a validation case with a test.

### 2026-09-12T01:37:36.110Z - Proof thread-safety set NOT_APPLICABLE: Pure function with no shared state.

### 2026-09-12T01:37:36.183Z - Proof configurability set NOT_APPLICABLE: The 256 KiB cap is deliberately a constant; a tunable cap would be an attack surface.
