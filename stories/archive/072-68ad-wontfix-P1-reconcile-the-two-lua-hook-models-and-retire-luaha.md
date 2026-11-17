---
id: 072-68ad
title: Reconcile the two Lua hook models and retire luahandler
status: wontfix
priority: P1
type: refactor
created: "2026-10-01T13:29:44.859Z"
updated: "2026-10-01T13:51:33.519Z"
dependencies: ["071-3816"]
plan: plans/main-divergence-integration.md
plan_step: Step 5
depends_on: ["stories/071-3816-pending-P1-port-the-lua-engine-and-scheduler-onto-the-trunk-s.md"]
completed_at: "2026-10-01T13:51:33.518Z"
---

# Reconcile the two Lua hook models and retire luahandler

## Problem Statement

Remote's internal/handlers/luahandler registers script handlers on built-in action names (script:join, script:inquire). The ported engine forbids a script claiming a built-in name and extends built-ins through indri.before/indri.after_action instead. The two models cannot coexist, so one must go — but luahandler provides real behaviour (indri.refresh, indri.refreshSelf, post-Mutate ordering, the inquire notification path) that must not be lost with it.

## Acceptance Criteria

- [ ] A script can hook join, leave, kick and inquire through indri.before and indri.after_action, proven by tests
- [ ] login, logout, reconnect and register remain unhookable and a script attempting it is refused
- [ ] indri.refresh() and indri.refreshSelf() exist in the engine with tests covering post-Mutate execution ordering
- [ ] The inquire notification path behaviour from InquireNotificationHandler is demonstrated by an engine test before luahandler is deleted
- [ ] internal/handlers/luahandler is removed only after the above tests are green
- [ ] A script may not claim a dispatch phase name (received, processed)
- [ ] VERIFY: go test ./internal/services/lua/ ./internal/handlers/router/

## Files

- internal/handlers/luahandler/
- internal/services/lua/hooks.go
- internal/services/lua/lifecycle.go

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

### 2026-10-01T13:51:33.453Z - Superseded by the trunk reversal: origin/main lacks the connection-independent dispatch refactor, so local main became the trunk and the port direction inverted. Plan plans/main-divergence-integration.md revised; this story's criteria were written for the opposite direction. Replaced by the revised story set.

