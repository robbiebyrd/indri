---
id: 071-6b19
title: The script dispatch tree is bounded by accident, not by its caps
status: ready
priority: P2
type: fix
created: "2026-09-13T23:19:58.241Z"
updated: "2026-09-13T23:20:01.594Z"
dependencies: []
plan: plans/lua-game-scripting.md
plan_step: Step 9
---

# The script dispatch tree is bounded by accident, not by its caps

## Problem Statement

Story 054 capped script-emitted events at depth 10 and fan-out 32, but its own worklog records that those caps do not bound the dispatch tree by themselves. The real bound is that every invocation context derives from the root messages 100ms budget, so the tree cannot outlive it. That is incidental rather than designed, and it stops holding the moment script.InvocationTimeout stops deriving from its caller - which is exactly the kind of change someone makes without knowing the timeout was load-bearing. A worklog note will not stop them; a test named after the invariant will.

## Acceptance Criteria

- [ ] A test asserts that a script invocation context is derived from its callers context, and fails if it is ever given an independent deadline
- [ ] The bound on the dispatch tree is stated in a comment where InvocationTimeout is defined, not only in a worklog
- [ ] A pathological script that sends the maximum fan-out at every depth is shown to terminate, with the mechanism that stops it named in the assertion
- [ ] VERIFY: go test -race ./internal/services/lua/ ./internal/handlers/actions/script/

## Files

- internal/handlers/actions/script/handler.go
- internal/services/lua/host_io.go

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

### 2026-09-13T23:21:06.135Z - Refinement from the session that found this, having just been in the code. (1) The invariant to name is NOT that the caps bound the tree - they do not, 32^10 is about 1.1e15. It is that every dispatched context derives from the root messages deadline, so the whole tree shares one 100ms budget. The test must assert the derivation, not the arithmetic. (2) The break is a two-character change: if script.Handler ever derives its deadline from context.Background() instead of req.Ctx(), every hop gets a fresh 100ms and the tree becomes unbounded in time as well as breadth - plausibly done by someone fixing a cancellation bug. The test should fail on exactly that substitution.

