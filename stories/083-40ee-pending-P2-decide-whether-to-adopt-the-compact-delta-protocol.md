---
id: 083-40ee
title: Decide whether to adopt the compact delta protocol
status: pending
priority: P2
type: task
created: "2026-10-01T13:52:29.872Z"
updated: "2026-10-01T13:52:39.842Z"
dependencies: ["080-cf52"]
plan: plans/main-divergence-integration.md
plan_step: Step 6
depends_on: ["stories/080-cf52-pending-P1-add-sqlite-and-postgres-game-backends-against-loca.md"]
---

# Decide whether to adopt the compact delta protocol

## Problem Statement

origin/main replaced the dotted-path delta with positional integer-array encoding over MessagePack, plus layout frames and keyframe-on-shape-change. It buys payload size but costs the client parser, every scripttest fixture, and local's escaped-key path support, since remote splits paths on a plain dot. This is a genuine decision, not a carry, and the plan's default is to decline it.

## Acceptance Criteria

- [ ] A written comparison of payload size against migration cost is produced before any code is written
- [ ] The decision is recorded in the story worklog naming explicitly what is given up
- [ ] If declined: local's dotted-path delta is retained and the compact protocol is filed as its own plan
- [ ] If adopted: SanitizeDelta's pair-array form strips exactly what the map form stripped, so it still agrees with GameService.Sanitize
- [ ] If adopted: escaped-key paths still round-trip, or the loss is recorded as accepted
- [ ] If adopted: every scripttest fixture and the client parser are updated and green
- [ ] game-state-parser.bench.node-test.ts is re-baselined either way

## Files

- internal/services/events/
- client/services/game-state-parser.ts
- internal/services/scripttest/

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

