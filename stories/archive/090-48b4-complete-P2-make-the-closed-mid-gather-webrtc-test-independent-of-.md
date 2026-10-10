---
id: 090-48b4
title: Make the closed-mid-gather WebRTC test independent of network egress
status: complete
priority: P2
type: fix
created: "2026-10-03T04:19:37.000Z"
updated: "2026-10-04T18:51:10.000Z"
dependencies: []
---

# Make the closed-mid-gather WebRTC test independent of network egress

## Problem Statement

TestAnswerOffer_ClosedMidGatherReturnsError (internal/transport/webrtc/peer_test.go) keeps ICE gathering open by pointing the server PeerConnection at an unreachable STUN server (203.0.113.1). That only works when outbound UDP is allowed: with egress denied, the STUN send fails at once, gathering completes, answerOffer succeeds before the close, and the test fails at peer_test.go:398. Reproduced deterministically inside the Claude Code sandbox (5/5 fail) and never outside it (30/30 pass). Any CI runner without outbound UDP would fail the same way. Pre-existing; found while verifying 081.

## Acceptance Criteria

- [x] The test holds gathering open without depending on network egress (e.g. a local UDP listener that swallows STUN requests, if pion will gather against it)
- [x] It passes with outbound UDP denied and with it allowed
- [x] Breaking answerOffer's closed-mid-gather error path still fails the test
- [x] VERIFY: go test -count=20 -run TestAnswerOffer_ClosedMidGatherReturnsError ./internal/transport/webrtc/

## Work Log

### 2026-10-04T18:51:10.000Z - FIX. The server PeerConnection now gathers against silentSTUN: a loopback UDP socket that reads every STUN request and never answers, so gathering stays open with no network at all, where the unroutable TEST-NET-3 address only did so when outbound UDP was allowed. pion does gather a server-reflexive candidate against a loopback STUN server, which is what keeps ICEGatheringStateComplete from firing. Verified: 10/10 pass inside the Claude Code sandbox (outbound UDP denied; the old test failed 5/5 there), 20/20 with network; with answerOffer's closed-mid-gather check removed, 10/10 runs fail. Full webrtc package green under -race.

