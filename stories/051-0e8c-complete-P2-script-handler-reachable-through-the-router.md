---
id: 051-0e8c
title: Script handler reachable through the router
status: complete
priority: P2
type: feature
created: "2026-09-12T01:27:37.173Z"
updated: "2026-09-13T18:04:15.198Z"
dependencies: ["050"]
plan: plans/lua-game-scripting.md
plan_step: Step 6
depends_on: ["stories/050-c257-pending-P2-script-loading-and-per-state-action-registration.md"]
completed_at: "2026-09-13T18:04:15.198Z"
---

# Script handler reachable through the router

## Problem Statement

Script-declared actions must reach clients through the existing router rather than a second dispatch mechanism, and an unregistered action is silently unreachable.

## Acceptance Criteria

- [x] A handler implementing actions.MessageHandler invokes the engine with a per-invocation timeout
- [x] boot.registerHandlers registers one router action per declared script action
- [x] LuaEngine is wired into the services injector
- [x] A test mirroring TestRegisterHandlers_CoversEveryActionPackage asserts every manifest action is reachable via router.Dispatch
- [x] Panics stay contained by the existing invokeHandler recover
- [x] VERIFY: go test ./internal/services/boot/ ./internal/handlers/...

## Files

- internal/handlers/actions/script/handler.go
- internal/services/boot/handlers.go
- internal/injector/services.go

## Proof

- [x] [completeness] Completeness (6 of 6 criteria met; the handler, the boot registration, the injector wiring and the coverage test all exist and go build/vet/test -race ./... is green)
- [x] [feature-availability] Feature availability (TestRegisterHandlers_CoversEveryScriptAction dispatches every declared action through router.Dispatch and requires the script's own marker back; mutation-checked by removing the registerScriptHandlers call, which fails it)
- [x] [robustness] Robustness (Invoke refuses an action no script declared, refuses a payload it cannot convert, and stops a script at its deadline discarding the interrupted state; each has a test)
- [x] [resilience] Resilience (The 100ms deadline derives from req.Ctx（） rather than Background, so client disconnect and shutdown reach a running script; TestInvoke_InheritsTheCallersCancellation covers it)
- [x] [security] Security (sessionToLua omits Session.Token and Session.ID; TestInvoke_HandsTheRequestToTheRegisteredHandler asserts both come through as nil. The action is fixed at registration, never read from the request)
- [x] [defense-in-depth] Defense in depth (TestDispatch_ContainsAPanicFromTheEngine proves the router's existing invokeHandler recover still converts a panic from script code into an error)
- [x] [input-validation] Input validation (requestToLua converts the payload through toLua and returns an error rather than raising on an unconvertible value; the session is read from the transport, never supplied by the client)
- [x] [thread-safety] Thread safety (Each invocation borrows one pooled state and releases it; TestInvoke_GivesEachInvocationItsOwnGlobals proves no state leaks between calls, and go test -race is green)
- [~] [configurability] Configurability (The action set comes from the script file the server was started with; the 100ms timeout is a constant and making it configurable is not in this story's scope)

## QA

None — covered by tests in internal/handlers/actions/script and internal/services/lua

## Work Log

### 2026-09-13T18:02:35.755Z - Added Engine.Invoke (internal/services/lua/invoke.go): borrows a pooled state, looks up the action's closure on that state, marshals the request (action, payload, and a session view without Token or ObjectID) into that state's tables, runs it through pooledState.call under the caller's deadline, and releases the state so an interrupted one is discarded. Added internal/handlers/actions/script/handler.go: one Handler per script action, holding the injector and the action it was registered for, layering a 100ms per-invocation deadline on req.Ctx() before calling the engine. Wired LuaEngine into injector.ServicesInjector (built last in GetServices from ScriptRepo.Get().Scripts, closed in boot.closeResources) and added boot.registerScriptHandlers, which registers 'lua_'+action per Engine.Actions() entry after the built-ins and no-ops on a partial injector. New TestRegisterHandlers_CoversEveryScriptAction dispatches every declared action through router.Dispatch and requires the script's own marker back, so a missing registration fails (verified RED by disabling the call). Script-package tests cover the timeout, cancellation inheritance, the engine-less injector, and panic containment through invokeHandler; lua tests cover request/session marshalling, unknown actions, unconvertible payloads, deadline kill plus state discard, per-invocation globals, and invocation after Close. The one directory scan change: handlers/actions/script is not an action, so TestRegisterHandlers_CoversEveryActionPackage and TestBuiltinActions_CoversEveryActionPackage now skip that directory by name. go build, go vet and go test -race ./... are green.


### 2026-09-13T18:04:13.883Z - Proof completeness set PROVEN: 6 of 6 criteria; Invoke on Engine, a Handler per declared action, injector wiring and engine shutdown

### 2026-09-13T18:04:13.954Z - Proof feature-availability set PROVEN: TestRegisterHandlers_CoversEveryScriptAction dispatches every manifest action through router.Dispatch and requires the scripts own marker back; verified RED by disabling registration, since Dispatch answers an unknown action silently

### 2026-09-13T18:04:14.024Z - Proof robustness set PROVEN: An interrupted state is closed rather than pooled via the existing release policy; the engine is resolved late so a partial injector cannot panic the registry tests

### 2026-09-13T18:04:14.097Z - Proof resilience set PROVEN: closeResources closes the engine on shutdown, and registration no-ops when the injector has no engine, covered by its own test

### 2026-09-13T18:04:14.175Z - Proof security set PROVEN: The session table passed to Lua carries only userId, gameId and teamId; the bearer Token and the session ObjectID used as the broadcast filter key are deliberately absent and pinned by a test

### 2026-09-13T18:04:14.252Z - Proof defense-in-depth set PROVEN: A 100ms per-invocation deadline sits well under the 10s Redis lock lease, so a slow script cannot outlive the lock protecting the game it is editing

### 2026-09-13T18:04:14.329Z - Proof input-validation set NOT_APPLICABLE: Payload validation happens at the marshalling boundary in story 048; this story only routes

### 2026-09-13T18:04:14.408Z - Proof thread-safety set PROVEN: Handlers are resolved on the borrowed state, not shared; pool acquire and release exercised under -race

### 2026-09-13T18:04:14.484Z - Proof configurability set NOT_APPLICABLE: The timeout is a constant until story 058 introduces configuration

### 2026-09-13T18:04:27.983Z - Proof completeness set PROVEN: 6 of 6 criteria met; the handler, the boot registration, the injector wiring and the coverage test all exist and go build/vet/test -race ./... is green

### 2026-09-13T18:04:28.069Z - Proof feature-availability set PROVEN: TestRegisterHandlers_CoversEveryScriptAction dispatches every declared action through router.Dispatch and requires the script's own marker back; mutation-checked by removing the registerScriptHandlers call, which fails it

### 2026-09-13T18:04:28.141Z - Proof robustness set PROVEN: Invoke refuses an action no script declared, refuses a payload it cannot convert, and stops a script at its deadline discarding the interrupted state; each has a test

### 2026-09-13T18:04:28.217Z - Proof resilience set PROVEN: The 100ms deadline derives from req.Ctx() rather than Background, so client disconnect and shutdown reach a running script; TestInvoke_InheritsTheCallersCancellation covers it

### 2026-09-13T18:04:28.306Z - Proof security set PROVEN: sessionToLua omits Session.Token and Session.ID; TestInvoke_HandsTheRequestToTheRegisteredHandler asserts both come through as nil. The action is fixed at registration, never read from the request

### 2026-09-13T18:04:28.387Z - Proof defense-in-depth set PROVEN: TestDispatch_ContainsAPanicFromTheEngine proves the router's existing invokeHandler recover still converts a panic from script code into an error

### 2026-09-13T18:04:28.467Z - Proof input-validation set PROVEN: requestToLua converts the payload through toLua and returns an error rather than raising on an unconvertible value; the session is read from the transport, never supplied by the client

### 2026-09-13T18:04:28.546Z - Proof thread-safety set PROVEN: Each invocation borrows one pooled state and releases it; TestInvoke_GivesEachInvocationItsOwnGlobals proves no state leaks between calls, and go test -race is green

### 2026-09-13T18:04:28.623Z - Proof configurability set NOT_APPLICABLE: The action set comes from the script file the server was started with; the 100ms timeout is a constant and making it configurable is not in this story's scope
