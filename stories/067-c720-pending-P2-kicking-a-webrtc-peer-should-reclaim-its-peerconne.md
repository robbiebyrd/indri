---
id: "067-c720"
title: "Kicking a WebRTC peer should reclaim its PeerConnection"
status: pending
priority: P2
type: fix
created: 2026-09-13T17:57:18.988Z
updated: 2026-09-13T17:57:18.988Z
dependencies: []
plan: "plans/webrtc-transport.md"
---

# Kicking a WebRTC peer should reclaim its PeerConnection

## Problem Statement

Registry.Disconnect closes the rtcConn, which stops traffic in both directions, but it does not cascade to pc.Close(). A kicked player's ICE agent, DTLS and SCTP resources therefore survive until that peer's own Failed callback fires or the pending TTL reclaims it, and the pending TTL only applies to peers that never connected. This is resource reclamation rather than access control - the player can no longer send or receive - but on a busy server a stream of kicks leaks PeerConnections for as long as ICE takes to notice. The transport holds the peer table, but Registry.Disconnect only knows about conns, so the two need connecting.

## Acceptance Criteria

- [ ] A kicked session's PeerConnection is closed, not only its conn
- [ ] The peer is removed from the transport's peer table so it stops counting toward MaxPeers
- [ ] Closing is idempotent and cannot race the peer's own Failed or Closed teardown
- [ ] A test asserts the PeerConnection reaches a closed state after a kick, not merely that the conn is closed
- [ ] VERIFY: go test -race ./internal/transport/webrtc/

## Files

- internal/transport/webrtc/webrtc.go
- internal/transport/webrtc/peer.go

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

