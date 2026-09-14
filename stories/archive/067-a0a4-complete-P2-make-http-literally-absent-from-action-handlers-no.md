---
id: 067-a0a4
title: Make http literally absent from action handlers, not refused at call time
status: complete
priority: P2
type: refactor
created: "2026-09-13T18:45:08.719Z"
updated: "2026-09-14T00:59:05.055Z"
dependencies: ["063"]
plan: plans/lua-game-scripting.md
plan_step: Step 14
depends_on: ["stories/063-e85b-pending-P2-game-lifecycle-events.md"]
completed_at: "2026-09-14T00:59:05.055Z"
---

# Make http literally absent from action handlers, not refused at call time

## Problem Statement

Story 059 ships indri.http as a table whose get refuses for the whole of a dispatched action. That is a permission check inside an always-present function, which is the exact pattern the capability design in Step 13 rejects: absence means unreachable, and a check means auditing every function body instead of glancing at what was injected. It was the right call at the time because there is no scheduled entry point yet to hand a different host table to, but once timers (057) and lifecycle events (063) exist the two views can be distinguished and http should simply not be on the handler view.

## Acceptance Criteria

- [x] A script granted http sees indri.http as nil inside a dispatched action handler
- [x] The same script sees a working indri.http inside a scheduled or lifecycle callback
- [x] scopeHostTable and bindScope build the two views rather than a call-time guard
- [x] The call-time refuseInAction guard is removed once absence replaces it, so there is one mechanism and not two
- [x] Existing SSRF, content-type, byte-cap and timeout behaviour is unchanged, proved by the existing tests still passing

## Files

- internal/services/lua/capability.go
- internal/services/lua/host_http.go

## Proof

- [x] [completeness] Completeness (5 of 5 criteria; absence in an action, presence in a lifecycle callback, the escape-route walk, and refuseInAction removed so there is one mechanism not two)
- [x] [feature-availability] Feature availability (A real fetch runs from player:joined, is written back through indri.mutate and read out of the store; the same script sees indri.http as nil inside a dispatched action)
- [x] [robustness] Robustness (Mutation-checked: making actionTrigger.hostView return the full view fails four tests, making lifecycleTrigger.hostView return the action view fails two)
- [x] [resilience] Resilience (Two tables are built once at load and frozen, never swapped per call - a field swap would be unsafe under pooling because a scoped table belongs to the state rather than the call, and an invocation interrupted by a deadline or panic never runs the code that would put it back)
- [x] [security] Security (Absence replaces a call-time check, so what a script can do is a glance at what was injected rather than an audit of every function body; 058's walk over _G, package.loaded, package.preload and metatable routes is reused and parameterised)
- [x] [defense-in-depth] Defense in depth (hostViewFor falls back to the narrower view when there is no invocation or no trigger, so an unrecognised call context loses the capability rather than gaining it)
- [x] [input-validation] Input validation (Every SSRF, redirect, scheme, content-type, byte-cap and timeout test is untouched and passing; only the assertion on the deleted guard's message changed)
- [x] [thread-safety] Thread safety (A lifecycle then action then lifecycle sequence on the same pooled state proves no leak in either direction; suite green under -race)
- [x] [configurability] Configurability (A capability now declares where it lives next to how it is built, so the action view is derived from the declaration rather than from a condition someone has to remember to update)

## QA

Verified independently: build, vet, gofmt and full suite green under -race. Confirmed refuseInAction is gone from the tree, hostView is a method on the trigger interface with one implementation per kind, and http is declared off the action view while assets stays on it. 5/5.

## Work Log

### 2026-09-14T00:57:46.634Z - Replaced the call-time refuseInAction guard with absence. A capability now declares whether it is on the action view (capability.inAction); scopeHostTable builds two frozen tables per granted script - action (host API + action-safe grants) and full (everything granted) - installing each capability value once and sharing it between them. The trigger interface gained hostView(hostViews): actionTrigger returns the action view, lifecycleTrigger the full one, so the choice is a method per kind and not a condition. scopedHandler resolves the view per invocation through the current invocation's trigger, so nothing is mutated on a pooled state. http is off the action view, assets on it; load time runs under the full view, as before. refuseInAction and its call in hostHTTPGet are gone. Tests: absence in an action (handler and inside indri.mutate), unreachable by walking _G/package.loaded/package.preload/metatables (058's walk, now parameterised by capability name), a working fetch from a player:joined handler written back through indri.mutate, and view selection across lifecycle->action->lifecycle on one pooled state. Mutation-checked: flipping either hostView fails the new tests. Every SSRF, redirect, content-type, byte-cap and timeout test is untouched and passing; the only existing test changed is the action-absence one, whose assertion named the removed guard's message. Note: a timer-fired action is an actionTrigger and therefore keeps the action view - deliberate, since it runs the same handler a client can dispatch and holds the same game lock. go build, go vet, gofmt and go test -race all green tree-wide.


### 2026-09-14T00:59:04.335Z - Proof completeness set PROVEN: 5 of 5 criteria; absence in an action, presence in a lifecycle callback, the escape-route walk, and refuseInAction removed so there is one mechanism not two

### 2026-09-14T00:59:04.410Z - Proof feature-availability set PROVEN: A real fetch runs from player:joined, is written back through indri.mutate and read out of the store; the same script sees indri.http as nil inside a dispatched action

### 2026-09-14T00:59:04.480Z - Proof robustness set PROVEN: Mutation-checked: making actionTrigger.hostView return the full view fails four tests, making lifecycleTrigger.hostView return the action view fails two

### 2026-09-14T00:59:04.556Z - Proof resilience set PROVEN: Two tables are built once at load and frozen, never swapped per call - a field swap would be unsafe under pooling because a scoped table belongs to the state rather than the call, and an invocation interrupted by a deadline or panic never runs the code that would put it back

### 2026-09-14T00:59:04.637Z - Proof security set PROVEN: Absence replaces a call-time check, so what a script can do is a glance at what was injected rather than an audit of every function body; 058's walk over _G, package.loaded, package.preload and metatable routes is reused and parameterised

### 2026-09-14T00:59:04.722Z - Proof defense-in-depth set PROVEN: hostViewFor falls back to the narrower view when there is no invocation or no trigger, so an unrecognised call context loses the capability rather than gaining it

### 2026-09-14T00:59:04.803Z - Proof input-validation set PROVEN: Every SSRF, redirect, scheme, content-type, byte-cap and timeout test is untouched and passing; only the assertion on the deleted guard's message changed

### 2026-09-14T00:59:04.886Z - Proof thread-safety set PROVEN: A lifecycle then action then lifecycle sequence on the same pooled state proves no leak in either direction; suite green under -race

### 2026-09-14T00:59:04.969Z - Proof configurability set PROVEN: A capability now declares where it lives next to how it is built, so the action view is derived from the declaration rather than from a condition someone has to remember to update
