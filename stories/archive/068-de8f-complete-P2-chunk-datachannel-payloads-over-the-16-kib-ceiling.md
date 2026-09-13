---
id: 068-de8f
title: Chunk DataChannel payloads over the 16 KiB ceiling
status: complete
priority: P2
type: feature
created: "2026-09-13T17:57:18.991Z"
updated: "2026-09-13T18:28:59.988Z"
dependencies: []
plan: plans/webrtc-transport.md
completed_at: "2026-09-13T18:28:59.988Z"
---

# Chunk DataChannel payloads over the 16 KiB ceiling

## Problem Statement

The practical cross-browser DataChannel message ceiling is 16 KiB, not the spec's 256 KiB, because of a Firefox and Chromium fragmentation incompatibility. Payloads above it are currently logged and dropped. A game keyframe can exceed 16 KiB, so a WebRTC client joining a large game silently never receives its initial state and sees a board that never updates. Dropping is better than truncating, but it is not a shipping answer.

## Acceptance Criteria

- [x] A payload over the ceiling is split, sent, and reassembled into the identical bytes on the client
- [x] The ceiling is read from RTCSctpTransport.maxMessageSize at runtime where available rather than hardcoded
- [x] Reassembly tolerates a connection closing mid-sequence without leaking the partial buffer
- [x] A payload under the ceiling is sent unchanged, with no framing overhead
- [x] The client half normalises chunks in the same single place binary payloads are already normalised
- [x] VERIFY: go test -race ./internal/transport/webrtc/

## Files

- internal/transport/webrtc/conn.go
- client/services/webrtc-transport.ts
- client/services/webrtc-signal.ts

## Proof

- [x] [completeness] Completeness (Five criteria with tests on both sides; go -race -count=5 and 346 client tests green.)
- [x] [feature-availability] Feature availability (A keyframe larger than the ceiling now arrives instead of being dropped, which is the whole point: a client joining a large game previously never received its initial state.)
- [x] [robustness] Robustness (Boundary cases covered: exactly at the ceiling, just over, and a multi-byte UTF-8 rune split across a chunk boundary on both sides, with the test asserting its own payload is valid UTF-8 first.)
- [x] [resilience] Resilience (A close mid-sequence stops the next chunk and discards the partial buffer; the reassembler is per-connection and bounded, so an incomplete sequence cannot grow without limit.)
- [~] [security] Security (No authentication or authorisation surface; framing only.)
- [x] [defense-in-depth] Defense in depth (Two independent guards on the buffer: it is dropped with the connection, and bounded in size while the connection lives.)
- [x] [input-validation] Input validation (The marker cannot collide with any payload the transport sends, verified against every write path rather than assumed, and a degenerate negotiated ceiling falls back rather than looping.)
- [x] [thread-safety] Thread safety (A single drain goroutine owns all writes and does not yield mid-sequence, which is what removes the need for sequence numbers; -race -count=5 clean.)
- [x] [configurability] Configurability (The ceiling is read from the negotiated SCTP capability at runtime, with the conservative 16 KiB only as a fallback.)

## Work Log

### 2026-09-13T18:27:14.133Z - Implemented chunk framing (NUL-byte marker + continue/final flag, 2-byte header) in internal/transport/webrtc/conn.go (send/sendChunked/writeRaw) and client/services/webrtc-signal.ts (ChunkReassembler + normaliseMessage). Ceiling read from pion's negotiated SCTP max-message-size via negotiatedMaxMessageSize(pc), falling back to 16 KiB. All 5 criteria verified with go test -race -count=5 and pnpm test/typecheck (clean reinstall).

### 2026-09-13T18:28:53.200Z - Chunk frames are two raw bytes prefixed to a slice of the payload: a 0x00 marker and a final flag. Every message this transport writes is a JSON object, whose first byte is always {, so the receiver tells a chunk from an ordinary message with one byte read and collision is impossible by construction rather than unlikely. No sequence number is needed because drain is the sole writer and sendChunked does not yield until a sequence finishes, which makes interleaving impossible. Ceiling comes from pion's negotiated SCTP capability, falling back to 16 KiB.


### 2026-09-13T18:28:53.273Z - Proof completeness set PROVEN: Five criteria with tests on both sides; go -race -count=5 and 346 client tests green.

### 2026-09-13T18:28:53.345Z - Proof feature-availability set PROVEN: A keyframe larger than the ceiling now arrives instead of being dropped, which is the whole point: a client joining a large game previously never received its initial state.

### 2026-09-13T18:28:53.449Z - Proof robustness set PROVEN: Boundary cases covered: exactly at the ceiling, just over, and a multi-byte UTF-8 rune split across a chunk boundary on both sides, with the test asserting its own payload is valid UTF-8 first.

### 2026-09-13T18:28:53.522Z - Proof resilience set PROVEN: A close mid-sequence stops the next chunk and discards the partial buffer; the reassembler is per-connection and bounded, so an incomplete sequence cannot grow without limit.

### 2026-09-13T18:28:53.602Z - Proof security set NOT_APPLICABLE: No authentication or authorisation surface; framing only.

### 2026-09-13T18:28:53.682Z - Proof defense-in-depth set PROVEN: Two independent guards on the buffer: it is dropped with the connection, and bounded in size while the connection lives.

### 2026-09-13T18:28:53.761Z - Proof input-validation set PROVEN: The marker cannot collide with any payload the transport sends, verified against every write path rather than assumed, and a degenerate negotiated ceiling falls back rather than looping.

### 2026-09-13T18:28:53.839Z - Proof thread-safety set PROVEN: A single drain goroutine owns all writes and does not yield mid-sequence, which is what removes the need for sequence numbers; -race -count=5 clean.

### 2026-09-13T18:28:53.918Z - Proof configurability set PROVEN: The ceiling is read from the negotiated SCTP capability at runtime, with the conservative 16 KiB only as a fallback.
