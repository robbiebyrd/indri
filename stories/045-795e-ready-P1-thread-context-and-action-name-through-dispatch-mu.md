---
id: 045-795e
title: Thread context and action name through dispatch, mutate and locks
status: ready
priority: P1
type: refactor
created: "2026-09-12T01:27:37.146Z"
updated: "2026-09-12T01:29:15.810Z"
dependencies: []
plan: plans/lua-game-scripting.md
plan_step: Step 0
---

# Thread context and action name through dispatch, mutate and locks

## Problem Statement

actions.Request carries no context and no action name, GameRepo.Mutate uses the boot-time stored context, and lock.InProcess.Acquire checks ctx.Err once then blocks uncancellably on a keyed mutex. A Lua invocation killed by its deadline would leave a goroutine stuck on Mongo or a game lock, and hooks could not see which action fired. Every later step assumes this.

## Acceptance Criteria

- [ ] actions.Request carries Context and Action, and router.Dispatch populates both
- [ ] GameRepo.Mutate takes a ctx parameter instead of dereferencing the boot-time stored context
- [ ] lock.InProcess.Acquire returns ctx.Err when the context is cancelled while already waiting on the keyed mutex, not only before
- [ ] Cancelling the context makes an in-flight Mutate return promptly instead of blocking
- [ ] The abandoned lock waiter is handed off and released rather than leaked
- [ ] VERIFY: go test -race ./internal/handlers/... ./internal/services/lock/ ./internal/repo/game/

## Files

- internal/handlers/actions/handler.go
- internal/handlers/router/act.go
- internal/repo/game/game.go
- internal/services/lock/inprocess.go

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

