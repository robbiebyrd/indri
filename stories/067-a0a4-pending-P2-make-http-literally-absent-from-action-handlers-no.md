---
id: 067-a0a4
title: Make http literally absent from action handlers, not refused at call time
status: pending
priority: P2
type: refactor
created: "2026-09-13T18:45:08.719Z"
updated: "2026-09-13T18:45:24.081Z"
dependencies: ["063"]
plan: plans/lua-game-scripting.md
plan_step: Step 14
depends_on: ["stories/063-e85b-pending-P2-game-lifecycle-events.md"]
---

# Make http literally absent from action handlers, not refused at call time

## Problem Statement

Story 059 ships indri.http as a table whose get refuses for the whole of a dispatched action. That is a permission check inside an always-present function, which is the exact pattern the capability design in Step 13 rejects: absence means unreachable, and a check means auditing every function body instead of glancing at what was injected. It was the right call at the time because there is no scheduled entry point yet to hand a different host table to, but once timers (057) and lifecycle events (063) exist the two views can be distinguished and http should simply not be on the handler view.

## Acceptance Criteria

- [ ] A script granted http sees indri.http as nil inside a dispatched action handler
- [ ] The same script sees a working indri.http inside a scheduled or lifecycle callback
- [ ] scopeHostTable and bindScope build the two views rather than a call-time guard
- [ ] The call-time refuseInAction guard is removed once absence replaces it, so there is one mechanism and not two
- [ ] Existing SSRF, content-type, byte-cap and timeout behaviour is unchanged, proved by the existing tests still passing

## Files

- internal/services/lua/capability.go
- internal/services/lua/host_http.go

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

