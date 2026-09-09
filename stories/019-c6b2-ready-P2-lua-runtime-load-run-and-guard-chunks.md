---
id: 019-c6b2
title: "Lua runtime: load, run and guard chunks"
status: ready
priority: P2
type: feature
created: "2026-09-09T19:04:59.223Z"
updated: "2026-09-09T19:05:27.826Z"
dependencies: ["012"]
plan: plans/layout-engine-renderer.md
plan_step: Step 9
depends_on: ["stories/012-8d40-pending-P1-spike-prove-fengari-bundles-and-runs-under-metro-o.md"]
---

# Lua runtime: load, run and guard chunks

## Problem Statement

Lua runs synchronously on the UI thread, so a runaway script freezes the app and a script error must never break rendering. The runtime needs error capture as values rather than throws, and a best-effort instruction budget. Depends on the fengari spike proving the platform first.

## Acceptance Criteria

- [ ] VERIFY: cd client && pnpm test
- [ ] runChunk returns a result value on both syntax and runtime errors instead of throwing, surfacing the Lua message
- [ ] A script error keeps the last good presentation and reports into a dev console overlay, mirroring the Go router's recover-per-handler stance
- [ ] An infinite loop is aborted by the instruction budget within the budget's order of magnitude
- [ ] A second script runs normally after a first one errored, proving no poisoned state
- [ ] io, os, require and load are nil inside the runtime
- [ ] The Lua stack is drained on both success and error paths, so a long session does not leak a stack slot per event
- [ ] Documentation states plainly that the instruction budget is best-effort, not a hard limit, because count hooks are per-coroutine and cannot interrupt inside a host function

## Files

- client/layout/lua/runtime.ts
- client/layout/lua/runtime.node-test.ts

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

