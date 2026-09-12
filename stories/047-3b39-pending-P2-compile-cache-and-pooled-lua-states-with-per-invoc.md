---
id: 047-3b39
title: Compile cache and pooled Lua states with per-invocation isolation
status: pending
priority: P2
type: feature
created: "2026-09-12T01:27:37.161Z"
updated: "2026-09-12T01:29:07.263Z"
dependencies: ["046"]
plan: plans/lua-game-scripting.md
plan_step: Step 2
depends_on: ["stories/046-f0ec-pending-P1-hardened-gopher-lua-state-with-a-stripped-standard.md"]
---

# Compile cache and pooled Lua states with per-invocation isolation

## Problem Statement

LState is not goroutine-safe and creating one per request is wasteful, but a pooled state carries globals and module tables between invocations, and an interrupted state carries a contaminated stack.

## Acceptance Criteria

- [ ] Chunks compile once to a FunctionProto and are shared across states, while LFunctions are never shared
- [ ] A handler runs with a fresh environment table whose __index falls through to the real globals, applied with SetFEnv
- [ ] A global assigned during one invocation is invisible to the next invocation on the same pooled state
- [ ] A state is discarded rather than returned to the pool after a deadline kill, an ApiErrorPanic or a stack overflow
- [ ] Required module tables are frozen so a script cannot leak mutations to the next invocation
- [ ] Fifty concurrent invocations pass under the race detector
- [ ] VERIFY: go test -race ./internal/services/lua/

## Files

- internal/services/lua/compile.go
- internal/services/lua/pool.go

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

