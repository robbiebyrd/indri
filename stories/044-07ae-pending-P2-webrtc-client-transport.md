---
id: 044-07ae
title: WebRTC client transport
status: pending
priority: P2
type: feature
created: "2026-09-12T01:27:30.685Z"
updated: "2026-09-12T01:27:41.878Z"
dependencies: ["043-5dc1"]
plan: plans/webrtc-transport.md
plan_step: Step 11
depends_on: ["stories/043-5dc1-pending-P2-extract-the-client-transport-seam.md"]
---

# WebRTC client transport

## Problem Statement

The client needs to gather ICE fully, post one offer and apply one answer, then carry game traffic on a reliable ordered DataChannel matching the delta contract. One code path has to serve web and native, which the web shim provides, and an undeclared dependency will pass locally while failing a clean CI install.

## Acceptance Criteria

- [ ] react-native-webrtc, the config plugin and the web shim are declared dependencies
- [ ] The client gathers ICE fully before posting its offer, matching the non-trickle server
- [ ] Separate game and signal DataChannels are created, with game reliable and ordered
- [ ] One code path serves web and native through the shim
- [ ] Binary and text payload handling is normalised so web and native produce the same JS type
- [ ] VERIFY: cd client && pnpm run typecheck && pnpm test

## Files

- client/services/webrtc-transport.ts
- client/services/webrtc-transport.node-test.ts
- client/package.json

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

