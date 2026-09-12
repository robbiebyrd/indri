---
id: 046-f0ec
title: Hardened gopher-lua state with a stripped standard library
status: pending
priority: P1
type: feature
created: "2026-09-12T01:27:37.160Z"
updated: "2026-09-12T01:29:07.183Z"
dependencies: ["045"]
plan: plans/lua-game-scripting.md
plan_step: Step 1
depends_on: ["stories/045-795e-pending-P1-thread-context-and-action-name-through-dispatch-mu.md"]
---

# Hardened gopher-lua state with a stripped standard library

## Problem Statement

gopher-lua ships no sandbox mode. Dangerous globals must be removed by hand after OpenBase, and the context deadline cannot interrupt a single long library call, so string.rep and string.format need their own caps.

## Acceptance Criteria

- [ ] gopher-lua v1.1.1 is added to go.mod and lua.Options uses only fields that exist (MaxCallStackSize does not exist and must not be referenced)
- [ ] OpenPackage is opened before OpenBase, and only package, base, table, string and math are opened
- [ ] dofile, loadfile, load, loadstring, collectgarbage, print, setmetatable, getmetatable, rawset, rawget and newproxy are all LNil after construction
- [ ] The os, io and debug tables are absent
- [ ] package.path and package.cpath are cleared and package.loadlib is nil
- [ ] A script that loops forever is killed by the context deadline
- [ ] string.rep and string.format are wrapped with output size caps so an allocation bomb errors instead of allocating
- [ ] VERIFY: go test ./internal/services/lua/

## Files

- internal/services/lua/sandbox.go
- go.mod

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

