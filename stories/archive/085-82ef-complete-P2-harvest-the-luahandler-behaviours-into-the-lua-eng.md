---
id: 085-82ef
title: Harvest the luahandler behaviours into the Lua engine
status: complete
priority: P2
type: feature
created: "2026-10-01T13:52:35.317Z"
updated: "2026-10-04T18:42:31.000Z"
dependencies: ["081-7e0a"]
plan: plans/main-divergence-integration.md
plan_step: Step 8
depends_on: ["stories/081-7e0a-pending-P1-port-the-player-slot-model-and-migrate-the-user-ke.md"]
---

# Harvest the luahandler behaviours into the Lua engine

## Problem Statement

origin/main's luahandler provides indri.refresh(), indri.refreshSelf(), post-Mutate execution ordering and an inquire notification path. Its registration model puts scripts on built-in action names, which local's engine forbids by design. The behaviours are worth having; the registration model is not.

## Acceptance Criteria

- [x] indri.refresh() and indri.refreshSelf() exist in internal/services/lua with tests covering post-Mutate execution ordering
- [x] The inquire notification behaviour is reachable through indri.before/indri.after_action and proven by a test
- [x] A script hooking join, leave, kick and inquire works through the hook API
- [x] login, logout, reconnect and register remain unhookable and a script attempting it is refused
- [x] A script still may not claim a built-in action name or a dispatch phase (received, processed)
- [x] No code is copied from luahandler; only behaviours are reimplemented against the engine's model
- [x] VERIFY: go test ./internal/services/lua/

## Files

- internal/services/lua/hooks.go
- internal/services/lua/lifecycle.go

## Proof

- [x] [completeness] Completeness
- [x] [feature-availability] Feature availability
- [x] [robustness] Robustness
- [x] [resilience] Resilience
- [x] [security] Security
- [x] [defense-in-depth] Defense in depth
- [x] [input-validation] Input validation
- [x] [thread-safety] Thread safety
- [x] [configurability] Configurability

## Work Log

### 2026-10-04T18:42:31.000Z - IMPLEMENTATION. Behaviours reimplemented on the engine's model; nothing from luahandler copied (it wrote to the connection directly and registered on built-in names). indri.refresh() and indri.refreshSelf() are effects on the existing ledger (host_refresh.go), so the post-Mutate ordering main got from a postMutateActions struct comes from the ledger's own guarantee: delivered after the handler returns and its write commits, once per commit, dropped with a write that failed; asking twice queues one. They reach the world through a new Keyframes dependency set at boot like Dispatch (injector/keyframes.go over GameService.GetJSONBytes and BroadcastService.Broadcast, both sanitized); nil, or a call with no game, refuses. The inquire notification needed no new code: inquire is hookable, and indri.after_action("inquire") runs after the inquiry's answer, sees its payload, and cannot mutate (no game) — proven end to end on handlertest with the real inquire handler and boot's own hook registration. join, leave, kick and inquire hooks load and register (new test); login/logout/reconnect/register refusal and the built-in / dispatch-phase guards were already covered (TestHooks_RefusedAtLoad, TestHooks_IndriOnStillRefusesABuiltin). SCRIPTING.md documents both.

### 2026-10-04T18:42:31.000Z - VERIFICATION. go build/vet/gofmt clean; go test -race -count=1 ./... green with MongoDB and PostgreSQL, 0 skips. Mutations: delivering a refresh inline instead of queueing it fails the committed-state, failed-write and refreshSelf tests; removing the once-per-invocation check fails the once test. The keyframe adapter is tested over real in-memory stores and a real connection registry: the broadcast reaches a session in the game, sanitized.

