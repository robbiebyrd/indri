---
id: 050-c257
title: Script loading and per-state action registration
status: complete
priority: P2
type: feature
created: "2026-09-12T01:27:37.167Z"
updated: "2026-09-13T17:51:20.101Z"
dependencies: ["047"]
plan: plans/lua-game-scripting.md
plan_step: Step 5
depends_on: ["stories/047-3b39-pending-P2-compile-cache-and-pooled-lua-states-with-per-invoc.md"]
completed_at: "2026-09-13T17:51:20.101Z"
---

# Script loading and per-state action registration

## Problem Statement

indri.on registers an anonymous closure that may capture upvalues. A FunctionProto cannot reconstruct upvalues and an LFunction is bound to the state that created it, so a single global handler map called on a pooled state is wrong.

## Acceptance Criteria

- [x] models.Script gains a scripts list resolved relative to the config file, and script loading returns errors instead of calling log.Fatalf
- [x] Each pooled state runs every chunk itself and owns its own handler map
- [x] The engine holds only the compiled protos and a frozen action manifest collected once at boot
- [x] Registering the same action twice fails at load, including across two different script files
- [x] A syntax error names the file and the line
- [x] The action names received and processed, plus every built-in action name, are reserved and refused at load
- [x] Boot fails loudly if two states disagree about which actions exist
- [x] VERIFY: go test ./internal/services/lua/

## Files

- internal/services/lua/engine.go
- internal/repo/script/script.go
- internal/models/config.go

## Proof

- [x] [completeness] Completeness (8 of 8 criteria; 18 engine tests plus 3 script-store tests, all table-driven with TempDir fixtures and no database)
- [~] [feature-availability] Feature availability (No Invoke API yet by design; story 051 wires the engine into the router and consumes handlersFor)
- [x] [robustness] Robustness (Chunks run under a 10s load deadline so a top-level infinite loop cannot hang boot; a syntax error names file and line; a missing file and a duplicate path are both refused explicitly)
- [x] [resilience] Resilience (Non-deterministic registration is detected per pooled state and fails boot rather than serving an inconsistent router, pinned by a test using math.random in the action name)
- [x] [security] Security (received, processed and all 11 built-in action names are refused at load, so a script cannot shadow login; the indri host table is frozen after registration)
- [x] [defense-in-depth] Defense in depth (Handlers live in the Lua registry rather than a Go map keyed by state, so they are invisible to script code and die with the state instead of leaking when the pool closes one)
- [x] [input-validation] Input validation (Empty and whitespace action names refused; duplicate registration refused across files with the first registration named by file and line)
- [x] [thread-safety] Thread safety (Each pooled state runs the chunks itself and owns its own handler closures; a test proves two states hold distinct LFunctions with independent upvalues)
- [x] [configurability] Configurability (models.Script gains a scripts list resolved against the config file directory rather than the working directory)

## QA

Verified independently: build, vet, full suite green under -race; no-database CI guard reports no unexpected skips. Confirmed log.Fatalf is gone from script.NewStore and that TestBuiltinActions_CoversEveryActionPackage holds the hardcoded reserved list against the actions directory in both directions. 8/8 criteria.

## Work Log

### 2026-09-13T17:49:40.198Z - Added internal/services/lua/engine.go: NewEngine compiles each listed script (path as chunk name, so syntax and runtime errors carry file:line), collects the action manifest once on a throwaway state, then builds a statePool whose prepare hook re-runs every chunk on each new state. Each state installs indri.on, builds its own handler map (closures cannot cross states), seals registration, freezes the indri table and stores the map in the state's Go-side registry, then is checked against the frozen manifest — a disagreement fails the build, so boot fails loudly. indri.on refuses duplicates across all files (naming the first registration's file:line), empty names, the dispatch phases received/processed and every built-in action; a test holds that reserved list against internal/handlers/actions. models.Script gained Scripts []string and internal/repo/script/script.go now returns wrapped errors instead of log.Fatalf, resolving relative script paths against the config file's directory. New tests: engine_test.go (18 tests, t.TempDir fixtures, no DB, no skips) and script_test.go. go build, go vet and go test -race all green over ./...


### 2026-09-13T17:51:19.141Z - Proof completeness set PROVEN: 8 of 8 criteria; 18 engine tests plus 3 script-store tests, all table-driven with TempDir fixtures and no database

### 2026-09-13T17:51:19.209Z - Proof feature-availability set NOT_APPLICABLE: No Invoke API yet by design; story 051 wires the engine into the router and consumes handlersFor

### 2026-09-13T17:51:19.282Z - Proof robustness set PROVEN: Chunks run under a 10s load deadline so a top-level infinite loop cannot hang boot; a syntax error names file and line; a missing file and a duplicate path are both refused explicitly

### 2026-09-13T17:51:19.354Z - Proof resilience set PROVEN: Non-deterministic registration is detected per pooled state and fails boot rather than serving an inconsistent router, pinned by a test using math.random in the action name

### 2026-09-13T17:51:19.429Z - Proof security set PROVEN: received, processed and all 11 built-in action names are refused at load, so a script cannot shadow login; the indri host table is frozen after registration

### 2026-09-13T17:51:19.507Z - Proof defense-in-depth set PROVEN: Handlers live in the Lua registry rather than a Go map keyed by state, so they are invisible to script code and die with the state instead of leaking when the pool closes one

### 2026-09-13T17:51:19.586Z - Proof input-validation set PROVEN: Empty and whitespace action names refused; duplicate registration refused across files with the first registration named by file and line

### 2026-09-13T17:51:19.666Z - Proof thread-safety set PROVEN: Each pooled state runs the chunks itself and owns its own handler closures; a test proves two states hold distinct LFunctions with independent upvalues

### 2026-09-13T17:51:19.742Z - Proof configurability set PROVEN: models.Script gains a scripts list resolved against the config file directory rather than the working directory
