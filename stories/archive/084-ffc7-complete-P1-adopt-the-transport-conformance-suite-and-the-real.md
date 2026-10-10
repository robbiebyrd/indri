---
id: 084-ffc7
title: Adopt the transport conformance suite and the real-store handler harness
status: complete
priority: P1
type: chore
created: "2026-10-01T13:52:35.315Z"
updated: "2026-10-03T05:47:58.000Z"
dependencies: ["078-ceca"]
plan: plans/main-divergence-integration.md
plan_step: Step 7
depends_on: ["stories/078-ceca-pending-P1-verified-baseline-on-an-integration-branch-off-loc.md"]
---

# Adopt the transport conformance suite and the real-store handler harness

## Problem Statement

Local has five transports each with its own hand-rolled near-duplicate test file. origin/main built one shared conformance suite run against every transport, plus a harness that tests handlers against real in-memory stores instead of fakes. Both are test-infrastructure wins worth taking regardless of any other decision.

## Acceptance Criteria

- [x] internal/transport/transporttest exists and its suite runs against every transport that holds connections — ws, graphql, sse and webrtc. (Restated: REST holds no connections, so a connection suite has nothing to run against; its routes are held to the action registry by TestRestRoutesMatchRegisteredActions.)
- [x] The suite covers ordered one-at-a-time delivery, concurrent-write safety, broadcast filtering by sessionId, and exactly-once disconnect after a kick
- [x] internal/handlers/handlertest exists and at least one handler test uses real in-memory stores rather than hand-rolled fakes
- [x] TestRestRoutesMatchRegisteredActions still passes and still fails when a registered action has no route
- [x] TestGraphQLMutationsMatchRegisteredActions still passes and still fails when a registered action has no mutation
- [x] VERIFY: go test ./internal/transport/...

## Files

- internal/transport/transporttest/
- internal/handlers/handlertest/

## Proof

- [x] [completeness] Completeness
- [x] [feature-availability] Feature availability
- [x] [robustness] Robustness
- [x] [resilience] Resilience
- [x] [security] Security
- [x] [defense-in-depth] Defense in depth
- [x] [input-validation] Input validation
- [x] [thread-safety] Thread safety
- [x] [configurability] Configurability

## Work Log

### 2026-10-03T05:47:58.000Z - IMPLEMENTATION. transporttest is written for local's transports rather than copied from main, whose suite assumed every transport is two-way and carried binary frames and ?debug=1 from the compact-delta protocol 083 declined. Two contracts: what every connection-holding transport keeps (a connection registers and is written to; concurrent writes all arrive; BroadcastFilter targets by sessionId and Broadcast reaches all; a kick — the conn closed twice, racing — ends it exactly once; a client hang-up ends it exactly once; closing the transport ends every connection), and, for two-way transports (ws, webrtc), one client's messages arrive in order one at a time and Disconnect fires exactly once after the conn reports closed. The suite tags conns with sessionId server-side, which works for transports that authenticate at connect and those that learn the session at login alike; its payloads are JSON because the GraphQL subscription relays raw JSON. Harnesses: sse (bearer stream), graphql (graphql-transport-ws subscription), ws (coder/websocket), webrtc (pion over the game channel). Running it found two transport defects, both fixed: ws Conns() returned melody's error once closed where every other transport returns none; webrtc Close tore down the UDP mux before its connections, so their closes went out over a dead socket — measured 6.4s vs 0.4s to end two connections (four runs each). The suite bounds the end of a connection (EndNotice, 3s, under the ICE timeout), but the old webrtc ordering did not reliably fail it (one run came in at 2.6s), so that fix is evidenced by measurement, not by a test. handlertest builds a real injector over in-memory stores with the production game, user, auth and session services (gameService.NewService now takes the script itself); join, which had no tests, gets five on it: seating and the session's record of it, rejoin keeping the slot, a missing team refused up front, a full team leaving the session out of the game, and the login scene for an unauthenticated caller. Existing transport tests were kept: they are not near-duplicates of the suite (ws had 95 lines of tests).

### 2026-10-03T05:47:58.000Z - VERIFICATION. go build/vet/gofmt clean; go test -race -count=1 ./... green with MongoDB and PostgreSQL, 0 skips; transport conformance x10 under -race green on all four. Suite mutations caught: Registry.BroadcastFilter ignoring its match (filter case fails on sse and graphql), RemoveSink never deregistering (kick, hang-up and filter cases fail). Parity tests: removing the leave REST route fails TestRestRoutesMatchRegisteredActions; removing the inquire mutation from the schema and regenerating fails TestGraphQLMutationsMatchRegisteredActions (removing leaveGame instead is caught earlier, by a test that calls the resolver by name and stops compiling). Generated code restored byte-identical. Process note: one mutation backup was taken in the sandboxed shell, whose TMPDIR differs from the unsandboxed one, so its restore failed; registry.go — untouched by this story — was restored from git and the tree scanned for leftover mutations; none.

### 2026-10-03T05:47:58.000Z - PROOF. completeness: four transports conform; handlers have a real-store harness in use. feature-availability: kick, broadcast and shutdown behave the same on every transport. robustness/resilience: shutdown ends WebRTC connections promptly; a closed ws transport reports no connections rather than an error. security: broadcast targeting by sessionId is held on every transport. defense-in-depth: parity tests proven to fail. input-validation: join refuses a missing team before writing. thread-safety: concurrent writes, racing kicks and one-at-a-time delivery under -race. configurability: n/a.

