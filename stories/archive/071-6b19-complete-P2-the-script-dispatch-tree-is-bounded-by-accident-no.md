---
id: 071-6b19
title: The script dispatch tree is bounded by accident, not by its caps
status: complete
priority: P2
type: fix
created: "2026-09-13T23:19:58.241Z"
updated: "2026-09-14T00:13:59.963Z"
dependencies: []
plan: plans/lua-game-scripting.md
plan_step: Step 9
completed_at: "2026-09-14T00:13:59.963Z"
---

# The script dispatch tree is bounded by accident, not by its caps

## Problem Statement

Story 054 capped script-emitted events at depth 10 and fan-out 32, but its own worklog records that those caps do not bound the dispatch tree by themselves. The real bound is that every invocation context derives from the root messages 100ms budget, so the tree cannot outlive it. That is incidental rather than designed, and it stops holding the moment script.InvocationTimeout stops deriving from its caller - which is exactly the kind of change someone makes without knowing the timeout was load-bearing. A worklog note will not stop them; a test named after the invariant will.

## Acceptance Criteria

- [x] A test asserts that a script invocation context is derived from its callers context, and fails if it is ever given an independent deadline
- [x] The bound on the dispatch tree is stated in a comment where InvocationTimeout is defined, not only in a worklog
- [x] A pathological script that sends the maximum fan-out at every depth is shown to terminate, with the mechanism that stops it named in the assertion
- [x] VERIFY: go test -race ./internal/services/lua/ ./internal/handlers/actions/script/

## Files

- internal/handlers/actions/script/handler.go
- internal/services/lua/host_io.go

## Proof

- [x] [completeness] Completeness (4 of 4 criteria; derivation asserted, the bound recorded where InvocationTimeout is defined, and the pathological case naming the mechanism that stops it)
- [x] [feature-availability] Feature availability (The pathological test drives a real lua.Engine through the real script.invoke, not a stub)
- [x] [robustness] Robustness (The substitution goes red in about 10ms rather than attempting 1.1e15 dispatches, because the dispatcher refuses any hop arriving with a deadline that is not the root's)
- [x] [resilience] Resilience (The load-bearing case is a caller whose budget is already shorter than the timeout - the derived deadline must equal the caller's exactly, which is the hop case at depth one and beyond)
- [~] [security] Security (No authorisation or input surface; this bounds resource use rather than access)
- [x] [defense-in-depth] Defense in depth (A go/parser test binds the doc comment from both ends: it must name each file and symbol, and that file must still declare it, so the reference breaks loudly when the code moves rather than rotting silently)
- [~] [input-validation] Input validation (No external input; the fixture drives the existing host API)
- [x] [thread-safety] Thread safety (Suite green under -race with 063 landing underneath throughout)
- [x] [configurability] Configurability (invoke takes an injectable timeout, so the pathological test runs on a 5ms budget rather than a real 100ms sleep - no real sleeps anywhere)

## QA

Verified independently: build, vet, gofmt and full suite green under -race. Substitution test confirmed: replacing req.Ctx() with context.Background() fails all four derivation cases plus the pathological test in about 10ms. Criterion 2 breaks loudly - renaming host_io.go fails the comment-binding test. 4/4.

## Work Log

### 2026-09-13T23:21:06.135Z - Refinement from the session that found this, having just been in the code. (1) The invariant to name is NOT that the caps bound the tree - they do not, 32^10 is about 1.1e15. It is that every dispatched context derives from the root messages deadline, so the whole tree shares one 100ms budget. The test must assert the derivation, not the arithmetic. (2) The break is a two-character change: if script.Handler ever derives its deadline from context.Background() instead of req.Ctx(), every hop gets a fresh 100ms and the tree becomes unbounded in time as well as breadth - plausibly done by someone fixing a cancellation bug. The test should fail on exactly that substitution.

### 2026-09-14T00:12:29.251Z - Named the invariant and made it fail on the edit that breaks it. (1) TestInvoke_DerivesTheInvocationContextFromItsCaller asserts derivation, not arithmetic: a caller whose budget is already shorter than InvocationTimeout must get exactly that deadline (the hop case); an expired caller must arrive expired; a cancelled caller must be cancelled *while the engine runs*; a caller with no deadline must still carry its own context values through. (2) The comment on InvocationTimeout now states that the caps permit 32^10 (~1.1e15) dispatches and that the bound is this timeout plus the derivation from req.Ctx(), naming internal/services/lua/invoke.go Engine.Invoke and internal/services/lua/host_io.go sendEffect.deliver as the code it rests on; TestInvocationTimeout_NamesTheCodeItsBoundDependsOn binds the comment to those files from both ends, verified red by renaming host_io.go. (3) TestSend_MaximumFanOutAtEveryDepthStopsOnTheSharedBudget drives a real engine through the real script.invoke: the fixture sends until indri.send refuses it, so every level fans out to the cap and reaches depth 10 (observed 32 at each of depths 1-9). The dispatcher asserts the mechanism directly - every hop carried exactly the root invocation's deadline - and refuses any hop handed one of its own, so the broken code fails in 10ms instead of running 1.1e15 dispatches. Substitution proof: replacing req.Ctx() with context.Background() turns all four derivation cases and the pathological test red (re-verified after story 063 landed in the lua package); reverted. Note: the pre-existing TestInvoke_InheritsTheCallersCancellation could NOT catch the substitution - it read ctx.Err() after invoke returned, and invoke's own deferred cancel makes that Canceled either way; it is folded into the new table as the 'caller that went away' case, now read during the call.


### 2026-09-14T00:13:58.732Z - Proof completeness set PROVEN: 4 of 4 criteria; derivation asserted, the bound recorded where InvocationTimeout is defined, and the pathological case naming the mechanism that stops it

### 2026-09-14T00:13:58.809Z - Proof feature-availability set PROVEN: The pathological test drives a real lua.Engine through the real script.invoke, not a stub

### 2026-09-14T00:13:58.883Z - Proof robustness set PROVEN: The substitution goes red in about 10ms rather than attempting 1.1e15 dispatches, because the dispatcher refuses any hop arriving with a deadline that is not the root's

### 2026-09-14T00:13:58.962Z - Proof resilience set PROVEN: The load-bearing case is a caller whose budget is already shorter than the timeout - the derived deadline must equal the caller's exactly, which is the hop case at depth one and beyond

### 2026-09-14T00:13:59.043Z - Proof security set NOT_APPLICABLE: No authorisation or input surface; this bounds resource use rather than access

### 2026-09-14T00:13:59.124Z - Proof defense-in-depth set PROVEN: A go/parser test binds the doc comment from both ends: it must name each file and symbol, and that file must still declare it, so the reference breaks loudly when the code moves rather than rotting silently

### 2026-09-14T00:13:59.204Z - Proof input-validation set NOT_APPLICABLE: No external input; the fixture drives the existing host API

### 2026-09-14T00:13:59.286Z - Proof thread-safety set PROVEN: Suite green under -race with 063 landing underneath throughout

### 2026-09-14T00:13:59.365Z - Proof configurability set PROVEN: invoke takes an injectable timeout, so the pathological test runs on a 5ms budget rather than a real 100ms sleep - no real sleeps anywhere
