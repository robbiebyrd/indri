---
id: 051-0e8c
title: Script handler reachable through the router
status: pending
priority: P2
type: feature
created: "2026-09-12T01:27:37.173Z"
updated: "2026-09-12T01:29:07.498Z"
dependencies: ["050"]
plan: plans/lua-game-scripting.md
plan_step: Step 6
depends_on: ["stories/050-c257-pending-P2-script-loading-and-per-state-action-registration.md"]
---

# Script handler reachable through the router

## Problem Statement

Script-declared actions must reach clients through the existing router rather than a second dispatch mechanism, and an unregistered action is silently unreachable.

## Acceptance Criteria

- [ ] A handler implementing actions.MessageHandler invokes the engine with a per-invocation timeout
- [ ] boot.registerHandlers registers one router action per declared script action
- [ ] LuaEngine is wired into the services injector
- [ ] A test mirroring TestRegisterHandlers_CoversEveryActionPackage asserts every manifest action is reachable via router.Dispatch
- [ ] Panics stay contained by the existing invokeHandler recover
- [ ] VERIFY: go test ./internal/services/boot/ ./internal/handlers/...

## Files

- internal/handlers/actions/script/handler.go
- internal/services/boot/handlers.go
- internal/injector/services.go

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

