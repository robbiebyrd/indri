---
id: 076-4402
title: Decide the client layout engine and board renderer
status: wontfix
priority: P1
type: refactor
created: "2026-10-01T13:29:44.861Z"
updated: "2026-10-01T13:51:34.191Z"
dependencies: ["075-c1d4"]
plan: plans/main-divergence-integration.md
plan_step: Step 9
depends_on: ["stories/075-c1d4-pending-P1-decide-the-server-layout-action-between-the-two-im.md"]
completed_at: "2026-10-01T13:51:34.191Z"
---

# Decide the client layout engine and board renderer

## Problem Statement

This is 44 of the 73 add/add conflicts: client/layout/** and client/components/board/** were both written from scratch on each side, covering grid math, schema, the style compiler, the widget registry, the Lua bridge, the editor and the pickers. The TypeScript validator must stay textually aligned with the Go one, so this cannot be settled independently of the server layout decision.

## Acceptance Criteria

- [ ] A diff summary per subsystem is produced with a recommendation before any file is overwritten
- [ ] The chosen TypeScript validator constants and AABB overlap test are textually identical to the Go side
- [ ] The Go half still rejects and the TypeScript half still clamps; no rule is enforced only on the client
- [ ] Widgets remain a map keyed by id, never an array
- [ ] game.data.layout remains the canonical layout location with nothing in scene.data
- [ ] [VISUAL] Editor drag and resize, palette docking and the config panel verified by hand in a running client
- [ ] Clean install passes the way CI does: rm -rf node_modules && pnpm install --frozen-lockfile && pnpm run typecheck && pnpm test

## Files

- client/layout/
- client/components/board/
- client/services/message-handler.ts

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

### 2026-10-01T13:34:40.968Z - Decision analysis dispatched (read-only, pre-branch): 9-subsystem comparison across client/layout and client/components/board, plus the Go/TS constants and AABB parity check that determines rework on the Go side. Also resolving whether local's failover supervisor and trunk's protocol-client are the same idea twice.

### 2026-10-01T13:51:34.127Z - Superseded by the trunk reversal: origin/main lacks the connection-independent dispatch refactor, so local main became the trunk and the port direction inverted. Plan plans/main-divergence-integration.md revised; this story's criteria were written for the opposite direction. Replaced by the revised story set.

