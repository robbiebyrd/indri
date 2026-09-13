---
id: 068-de8f
title: Chunk DataChannel payloads over the 16 KiB ceiling
status: ready
priority: P2
type: feature
created: "2026-09-13T17:57:18.991Z"
updated: "2026-09-13T18:11:00.896Z"
dependencies: []
plan: plans/webrtc-transport.md
---

# Chunk DataChannel payloads over the 16 KiB ceiling

## Problem Statement

The practical cross-browser DataChannel message ceiling is 16 KiB, not the spec's 256 KiB, because of a Firefox and Chromium fragmentation incompatibility. Payloads above it are currently logged and dropped. A game keyframe can exceed 16 KiB, so a WebRTC client joining a large game silently never receives its initial state and sees a board that never updates. Dropping is better than truncating, but it is not a shipping answer.

## Acceptance Criteria

- [ ] A payload over the ceiling is split, sent, and reassembled into the identical bytes on the client
- [ ] The ceiling is read from RTCSctpTransport.maxMessageSize at runtime where available rather than hardcoded
- [ ] Reassembly tolerates a connection closing mid-sequence without leaking the partial buffer
- [ ] A payload under the ceiling is sent unchanged, with no framing overhead
- [ ] The client half normalises chunks in the same single place binary payloads are already normalised
- [ ] VERIFY: go test -race ./internal/transport/webrtc/

## Files

- internal/transport/webrtc/conn.go
- client/services/webrtc-transport.ts
- client/services/webrtc-signal.ts

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

