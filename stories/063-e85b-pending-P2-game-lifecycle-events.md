---
id: 063-e85b
title: Game lifecycle events
status: pending
priority: P2
type: feature
created: "2026-09-12T01:28:39.044Z"
updated: "2026-09-12T01:29:08.715Z"
dependencies: ["057"]
plan: plans/lua-game-scripting.md
plan_step: Step 19
depends_on: ["stories/057-1deb-pending-P2-indri-after-indri-at-and-indri-cancel.md"]
---

# Game lifecycle events

## Problem Statement

Step 14 gates the http capability to scheduled and lifecycle scripts, but no lifecycle event exists anywhere in the plan or the repo. models.Game has no status or ended field at all, so this is a dangling reference that must be closed.

## Acceptance Criteria

- [ ] Scripts can subscribe to game created, player joined, player left and scene changed
- [ ] Each event carries the game id and the relevant subject
- [ ] Events are emitted from the existing Go call sites through the deferred queue and never inline
- [ ] Events are emitted only after the originating mutation commits, so a handler never observes uncommitted state
- [ ] A lifecycle handler can call indri.mutate
- [ ] A lifecycle error does not fail the originating built-in action
- [ ] Lifecycle events carry no session, matching the timer contract
- [ ] An unknown lifecycle name is refused at load
- [ ] VERIFY: go test -race ./internal/services/lua/

## Files

- internal/services/lua/lifecycle.go
- internal/services/game/game.go

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

