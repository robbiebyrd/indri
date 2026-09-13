---
id: 067-c720
title: Kicking a WebRTC peer should reclaim its PeerConnection
status: complete
priority: P2
type: fix
created: "2026-09-13T17:57:18.988Z"
updated: "2026-09-13T18:11:51.918Z"
dependencies: []
plan: plans/webrtc-transport.md
started_at: "2026-09-13T18:08:04.649Z"
completed_at: "2026-09-13T18:11:51.917Z"
---

# Kicking a WebRTC peer should reclaim its PeerConnection

## Problem Statement

Registry.Disconnect closes the rtcConn, which stops traffic in both directions, but it does not cascade to pc.Close(). A kicked player's ICE agent, DTLS and SCTP resources therefore survive until that peer's own Failed callback fires or the pending TTL reclaims it, and the pending TTL only applies to peers that never connected. This is resource reclamation rather than access control - the player can no longer send or receive - but on a busy server a stream of kicks leaks PeerConnections for as long as ICE takes to notice. The transport holds the peer table, but Registry.Disconnect only knows about conns, so the two need connecting.

## Acceptance Criteria

- [x] A kicked session's PeerConnection is closed, not only its conn
- [x] The peer is removed from the transport's peer table so it stops counting toward MaxPeers
- [x] Closing is idempotent and cannot race the peer's own Failed or Closed teardown
- [x] A test asserts the PeerConnection reaches a closed state after a kick, not merely that the conn is closed
- [x] VERIFY: go test -race ./internal/transport/webrtc/

## Files

- internal/transport/webrtc/webrtc.go
- internal/transport/webrtc/peer.go

## Proof

- [x] [completeness] Completeness (Four criteria each with an assertion; -race -count=5 green with no flakes.)
- [x] [feature-availability] Feature availability (A kicked peer's PeerConnection now reaches Closed and the peer stops counting toward MaxPeers.)
- [x] [robustness] Robustness (The test asserts on pion's own SignalingState rather than internal bookkeeping, so it proves pion actually released the connection.)
- [x] [resilience] Resilience (Teardown remains idempotent under the existing concurrent-callback race test, which still passes.)
- [~] [security] Security (Resource reclamation, not access control: a kicked player already could neither send nor receive.)
- [x] [defense-in-depth] Defense in depth (Two layers survive: sync.Once on peer teardown, and pion's own idempotent Close beneath it.)
- [~] [input-validation] Input validation (No external input involved.)
- [x] [thread-safety] Thread safety (The reentrancy hazard was the point of the fix; pion dispatches DataChannel.OnClose on a fresh goroutine, so the remaining path unwinds rather than deadlocking. -race -count=5 clean.)
- [~] [configurability] Configurability (No configuration introduced.)

## Sub-Tasks
- [~] [T1] Anonymous connections carry no session key (seeded onto this story in error by a peer session; these are the eleven steps of the WebRTC plan, all already delivered, not sub-tasks of this follow-up)
- [~] [T2] Signal envelope decoding (seeded onto this story in error by a peer session; these are the eleven steps of the WebRTC plan, all already delivered, not sub-tasks of this follow-up)
- [~] [T3] DataChannel-backed connection (seeded onto this story in error by a peer session; these are the eleven steps of the WebRTC plan, all already delivered, not sub-tasks of this follow-up)
- [~] [T4] Peer factory and the non-trickle answer (seeded onto this story in error by a peer session; these are the eleven steps of the WebRTC plan, all already delivered, not sub-tasks of this follow-up)
- [~] [T5] Peer lifecycle and teardown (seeded onto this story in error by a peer session; these are the eleven steps of the WebRTC plan, all already delivered, not sub-tasks of this follow-up)
- [~] [T6] The transport and its signalling route (seeded onto this story in error by a peer session; these are the eleven steps of the WebRTC plan, all already delivered, not sub-tasks of this follow-up)
- [~] [T7] Wiring, config and the message-size guard (seeded onto this story in error by a peer session; these are the eleven steps of the WebRTC plan, all already delivered, not sub-tasks of this follow-up)
- [~] [T8] Spike — can this PeerConnection carry a track added later? (seeded onto this story in error by a peer session; these are the eleven steps of the WebRTC plan, all already delivered, not sub-tasks of this follow-up)
- [~] [T9] Spike — react-native-webrtc under Expo SDK 53 New Architecture (seeded onto this story in error by a peer session; these are the eleven steps of the WebRTC plan, all already delivered, not sub-tasks of this follow-up)
- [~] [T10] Extract the client transport seam (seeded onto this story in error by a peer session; these are the eleven steps of the WebRTC plan, all already delivered, not sub-tasks of this follow-up)
- [~] [T11] WebRTC client transport (seeded onto this story in error by a peer session; these are the eleven steps of the WebRTC plan, all already delivered, not sub-tasks of this follow-up)

## Work Log


### 2026-09-13T18:09:47.329Z - Sub-task T1 skipped: seeded onto this story in error by a peer session; these are the eleven steps of the WebRTC plan, all already delivered, not sub-tasks of this follow-up

### 2026-09-13T18:09:47.413Z - Sub-task T2 skipped: seeded onto this story in error by a peer session; these are the eleven steps of the WebRTC plan, all already delivered, not sub-tasks of this follow-up

### 2026-09-13T18:09:47.496Z - Sub-task T3 skipped: seeded onto this story in error by a peer session; these are the eleven steps of the WebRTC plan, all already delivered, not sub-tasks of this follow-up

### 2026-09-13T18:09:47.577Z - Sub-task T4 skipped: seeded onto this story in error by a peer session; these are the eleven steps of the WebRTC plan, all already delivered, not sub-tasks of this follow-up

### 2026-09-13T18:09:47.653Z - Sub-task T5 skipped: seeded onto this story in error by a peer session; these are the eleven steps of the WebRTC plan, all already delivered, not sub-tasks of this follow-up

### 2026-09-13T18:09:47.734Z - Sub-task T6 skipped: seeded onto this story in error by a peer session; these are the eleven steps of the WebRTC plan, all already delivered, not sub-tasks of this follow-up

### 2026-09-13T18:09:47.914Z - Sub-task T7 skipped: seeded onto this story in error by a peer session; these are the eleven steps of the WebRTC plan, all already delivered, not sub-tasks of this follow-up

### 2026-09-13T18:09:47.995Z - Sub-task T8 skipped: seeded onto this story in error by a peer session; these are the eleven steps of the WebRTC plan, all already delivered, not sub-tasks of this follow-up

### 2026-09-13T18:09:48.076Z - Sub-task T9 skipped: seeded onto this story in error by a peer session; these are the eleven steps of the WebRTC plan, all already delivered, not sub-tasks of this follow-up

### 2026-09-13T18:09:48.156Z - Sub-task T10 skipped: seeded onto this story in error by a peer session; these are the eleven steps of the WebRTC plan, all already delivered, not sub-tasks of this follow-up

### 2026-09-13T18:09:48.229Z - Sub-task T11 skipped: seeded onto this story in error by a peer session; these are the eleven steps of the WebRTC plan, all already delivered, not sub-tasks of this follow-up

### 2026-09-13T18:09:59.431Z - Closing a conn now tears down its peer, wired once in attachGame so the invariant holds for every path rather than just kick. Registry is shared by all four transports, so a webrtc-specific override there would be the wrong layer. rtcConn.Close gained a hook-free sibling closeConn that peer.close uses: sync.Once.Do is not reentrant-safe, so peer.close calling a conn.Close that called back into peer.teardown.Do would deadlock rather than be skipped.


### 2026-09-13T18:09:59.523Z - Proof completeness set PROVEN: Four criteria each with an assertion; -race -count=5 green with no flakes.

### 2026-09-13T18:09:59.592Z - Proof feature-availability set PROVEN: A kicked peer's PeerConnection now reaches Closed and the peer stops counting toward MaxPeers.

### 2026-09-13T18:09:59.665Z - Proof robustness set PROVEN: The test asserts on pion's own SignalingState rather than internal bookkeeping, so it proves pion actually released the connection.

### 2026-09-13T18:09:59.745Z - Proof resilience set PROVEN: Teardown remains idempotent under the existing concurrent-callback race test, which still passes.

### 2026-09-13T18:09:59.828Z - Proof security set NOT_APPLICABLE: Resource reclamation, not access control: a kicked player already could neither send nor receive.

### 2026-09-13T18:09:59.913Z - Proof defense-in-depth set PROVEN: Two layers survive: sync.Once on peer teardown, and pion's own idempotent Close beneath it.

### 2026-09-13T18:09:59.991Z - Proof input-validation set NOT_APPLICABLE: No external input involved.

### 2026-09-13T18:10:00.075Z - Proof thread-safety set PROVEN: The reentrancy hazard was the point of the fix; pion dispatches DataChannel.OnClose on a fresh goroutine, so the remaining path unwinds rather than deadlocking. -race -count=5 clean.

### 2026-09-13T18:10:00.155Z - Proof configurability set NOT_APPLICABLE: No configuration introduced.
