---
id: 036-f0c7
title: DataChannel-backed transport connection
status: complete
priority: P2
type: feature
created: "2026-09-12T01:27:30.674Z"
updated: "2026-09-12T01:37:40.997Z"
dependencies: []
plan: plans/webrtc-transport.md
plan_step: Step 3
completed_at: "2026-09-12T01:37:40.997Z"
---

# DataChannel-backed transport connection

## Problem Statement

pion documents no goroutine-safety guarantee for DataChannel.Send, so writes must be serialised through one owning goroutine. That goroutine is also the right place to honour BufferedAmount, so a slow peer backs up in its own bounded queue rather than piling into SCTP or stalling fan-out to everyone else.

## Acceptance Criteria

- [x] The conn embeds transport.BufferedConn so it inherits the shared drop-oldest queue
- [x] Writes reach the sink in order
- [x] The seventeenth queued message is dropped rather than blocking the broadcaster
- [x] Close stops the drain goroutine, proven by a done channel or goroutine count
- [x] Calling Close twice is safe
- [x] A sink reporting high BufferedAmount parks the drain instead of continuing to send
- [x] Tests drive a fake sink and need no real PeerConnection
- [x] VERIFY: go test -race ./internal/transport/webrtc/

## Files

- internal/transport/webrtc/conn.go
- internal/transport/webrtc/conn_test.go

## Proof

- [x] [completeness] Completeness (All seven numbered criteria have a dedicated test; go test -race -count=3 is clean with no flakes.)
- [~] [feature-availability] Feature availability (Not wired into a transport until story 039-233f.)
- [x] [robustness] Robustness (Double Close, a full queue and a parked drain are each covered by a test.)
- [x] [resilience] Resilience (A slow peer drops its oldest queued messages rather than stalling fan-out to other players, and the drain goroutine exits on Close.)
- [~] [security] Security (No authentication, authorisation or external input at this layer.)
- [x] [defense-in-depth] Defense in depth (Two independent backpressure layers: the bounded drop-oldest queue, and parking on BufferedAmount so nothing piles into SCTP.)
- [~] [input-validation] Input validation (Payloads come from the broadcaster and are already sanitised upstream by events.SanitizeDelta.)
- [x] [thread-safety] Thread safety (A single owning drain goroutine is the only caller of sink.Send, which is why the design exists; go test -race -count=3 is clean.)
- [~] [configurability] Configurability (Buffer size and threshold are constants matching sse.streamBuffer.)

## QA

go test -race -count=3 ./internal/transport/webrtc/ green, no flakes; verified independently by the parent session.

## Work Log

### 2026-09-12T01:36:39.251Z - rtcConn embeds transport.BufferedConn and adds a single owning drain goroutine, the only caller of sink.Send, which parks on OnBufferedAmountLow rather than polling when BufferedAmount is high. No pion import: the dataSink interface keeps tests free of a real PeerConnection.


### 2026-09-12T01:37:36.265Z - Proof completeness set PROVEN: All seven numbered criteria have a dedicated test; go test -race -count=3 is clean with no flakes.

### 2026-09-12T01:37:36.348Z - Proof feature-availability set NOT_APPLICABLE: Not wired into a transport until story 039-233f.

### 2026-09-12T01:37:36.421Z - Proof robustness set PROVEN: Double Close, a full queue and a parked drain are each covered by a test.

### 2026-09-12T01:37:36.496Z - Proof resilience set PROVEN: A slow peer drops its oldest queued messages rather than stalling fan-out to other players, and the drain goroutine exits on Close.

### 2026-09-12T01:37:36.572Z - Proof security set NOT_APPLICABLE: No authentication, authorisation or external input at this layer.

### 2026-09-12T01:37:36.655Z - Proof defense-in-depth set PROVEN: Two independent backpressure layers: the bounded drop-oldest queue, and parking on BufferedAmount so nothing piles into SCTP.

### 2026-09-12T01:37:36.737Z - Proof input-validation set NOT_APPLICABLE: Payloads come from the broadcaster and are already sanitised upstream by events.SanitizeDelta.

### 2026-09-12T01:37:36.812Z - Proof thread-safety set PROVEN: A single owning drain goroutine is the only caller of sink.Send, which is why the design exists; go test -race -count=3 is clean.

### 2026-09-12T01:37:36.885Z - Proof configurability set NOT_APPLICABLE: Buffer size and threshold are constants matching sse.streamBuffer.
