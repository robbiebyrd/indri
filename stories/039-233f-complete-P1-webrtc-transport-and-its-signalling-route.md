---
id: 039-233f
title: WebRTC transport and its signalling route
status: complete
priority: P1
type: feature
created: "2026-09-12T01:27:30.677Z"
updated: "2026-09-12T03:10:20.535Z"
dependencies: ["034-e42b", "036-f0c7", "038-b855"]
plan: plans/webrtc-transport.md
plan_step: Step 6
depends_on: ["stories/034-e42b-pending-P2-anonymous-connections-carry-no-session-key.md", "stories/036-f0c7-pending-P2-datachannel-backed-transport-connection.md", "stories/038-b855-pending-P1-peer-lifecycle-and-teardown.md"]
completed_at: "2026-09-12T03:10:20.534Z"
---

# WebRTC transport and its signalling route

## Problem Statement

WebRTC needs to stand alone as a transport, so signalling lives in the transport rather than the action registry. That allows an anonymous peer whose session binds when login arrives over the DataChannel, but it also means unauthenticated callers can allocate PeerConnections in a loop, so a TTL and a cap are part of the feature rather than polish.

## Acceptance Criteria

- [x] POST to the signalling route with a pion-generated offer returns an answer and registers a conn
- [x] A request carrying a valid bearer token binds SessionIDKey at handshake so BroadcastFilter reaches it
- [x] A request with no token yields an anonymous conn carrying no session key
- [x] A login sent over the DataChannel binds the session through the existing dispatch path, with no new action code
- [x] A peer that never reaches connected is closed and dropped after the pending TTL
- [x] Exceeding the global peer cap returns 503 rather than allocating
- [x] Disconnect closes the peer, reaching it across transports through the aggregate
- [x] The game and signal DataChannels are routed by label
- [x] Messages on the signal channel never reach the action router
- [x] VERIFY: go test -race ./internal/transport/webrtc/

## Files

- internal/transport/webrtc/webrtc.go
- internal/transport/webrtc/webrtc_test.go

## Proof

- [x] [completeness] Completeness (Ten tests green under -race -count=3, driving a real pion client in-process through the HTTP route.)
- [x] [feature-availability] Feature availability (A client can connect, authenticate at handshake or stay anonymous, send actions and receive targeted broadcasts.)
- [x] [robustness] Robustness (Peer cap, pending TTL, kick across the aggregate, and a kicked peer attempting to send are each covered.)
- [x] [resilience] Resilience (Disconnected is not terminal, teardown is once-only, and a peer that never connects is reclaimed by the TTL.)
- [x] [security] Security (Authorisation comes from the caller's own bearer token; anonymous peers carry no session key. A kicked peer can no longer reach the action router, which a regression test reproduced before the fix.)
- [x] [defense-in-depth] Defense in depth (Three layers: the cap reserves a slot before allocating so a check-then-act race cannot exceed it, the TTL reclaims peers that never connect, and the closed-conn guard stops inbound from a kicked peer.)
- [x] [input-validation] Input validation (Offers are validated by DecodeSignal before pion sees them, and the signal channel registers no message handler so nothing on it can reach the router.)
- [x] [thread-safety] Thread safety (Peer map guarded by a mutex, conn attachment guarded by connMu added in this story, teardown by sync.Once; -race -count=3 clean.)
- [x] [configurability] Configurability (PendingTTL and MaxPeers are exported Config fields following sse.Transport.Heartbeat, so tests use short values rather than sleeping for production defaults.)

## Work Log

### 2026-09-12T03:10:13.827Z - Transport, signalling route, label routing, pending-peer TTL and global cap. Found and fixed a security defect while verifying: Registry.Disconnect closed the conn but pion's DataChannel stayed open, so a kicked player stopped receiving yet kept sending into the action router. Guarded OnMessage on conn.IsClosed with a regression test that reproduced it first.


### 2026-09-12T03:10:13.899Z - Proof completeness set PROVEN: Ten tests green under -race -count=3, driving a real pion client in-process through the HTTP route.

### 2026-09-12T03:10:13.972Z - Proof feature-availability set PROVEN: A client can connect, authenticate at handshake or stay anonymous, send actions and receive targeted broadcasts.

### 2026-09-12T03:10:14.039Z - Proof robustness set PROVEN: Peer cap, pending TTL, kick across the aggregate, and a kicked peer attempting to send are each covered.

### 2026-09-12T03:10:14.112Z - Proof resilience set PROVEN: Disconnected is not terminal, teardown is once-only, and a peer that never connects is reclaimed by the TTL.

### 2026-09-12T03:10:14.188Z - Proof security set PROVEN: Authorisation comes from the caller's own bearer token; anonymous peers carry no session key. A kicked peer can no longer reach the action router, which a regression test reproduced before the fix.

### 2026-09-12T03:10:14.266Z - Proof defense-in-depth set PROVEN: Three layers: the cap reserves a slot before allocating so a check-then-act race cannot exceed it, the TTL reclaims peers that never connect, and the closed-conn guard stops inbound from a kicked peer.

### 2026-09-12T03:10:14.343Z - Proof input-validation set PROVEN: Offers are validated by DecodeSignal before pion sees them, and the signal channel registers no message handler so nothing on it can reach the router.

### 2026-09-12T03:10:14.420Z - Proof thread-safety set PROVEN: Peer map guarded by a mutex, conn attachment guarded by connMu added in this story, teardown by sync.Once; -race -count=3 clean.

### 2026-09-12T03:10:14.497Z - Proof configurability set PROVEN: PendingTTL and MaxPeers are exported Config fields following sse.Transport.Heartbeat, so tests use short values rather than sleeping for production defaults.
