---
id: 058-6453
title: Per-script capability injection
status: complete
priority: P2
type: feature
created: "2026-09-12T01:28:12.573Z"
updated: "2026-09-13T18:27:49.337Z"
dependencies: ["051"]
plan: plans/lua-game-scripting.md
plan_step: Step 13
depends_on: ["stories/051-0e8c-pending-P2-script-handler-reachable-through-the-router.md"]
completed_at: "2026-09-13T18:27:49.337Z"
---

# Per-script capability injection

## Problem Statement

A shared namespace with internal permission checks makes auditing what a script can do a read of every function body. Absent means unreachable is the stronger property. config.json also has no validation of any kind today.

## Acceptance Criteria

- [x] A script without a grant sees the capability table as nil rather than a permission check inside an always-present function
- [x] Grants are per-script, so one script grant is invisible to another loaded in the same process
- [x] An ungranted capability cannot be reached by walking the globals table or a shared metatable
- [x] An unknown grant name fails boot with a named error rather than silently granting nothing
- [x] config.json gains a schema version field so a later format change is detectable
- [x] An empty or missing scripts list warns loudly at boot rather than silently serving no actions
- [x] VERIFY: go test ./internal/services/lua/

## Files

- internal/services/lua/capability.go

## Proof

- [x] [completeness] Completeness (6 of 6 criteria; injection, absence, per-script isolation, read-only scoped tables and three distinct boot refusals all covered without a database)
- [x] [feature-availability] Feature availability (Boot resolves grants from config.json through NewEngineWithGrants; http and assets are registered names whose installers arrive in story 059)
- [x] [robustness] Robustness (scopedHandler rebinds every handler a granted chunk registered, because pooledState.call installs a fresh environment falling back to the shared globals - without it a capability works at load time and is nil the moment a player triggers the handler)
- [x] [resilience] Resilience (An ungranted script keeps the shared table and gets no wrapper, so the pre-existing load path is unchanged and a capability fault cannot affect scripts that were granted nothing)
- [x] [security] Security (An ungranted capability is absent from indri, _G, package.loaded, package.preload and every reachable metatable rather than present behind a permission check; the walk is a test and leaking the capability onto the shared table was mutation-checked to fail)
- [x] [defense-in-depth] Defense in depth (Scoped tables are frozen parent and children, and per-invocation global isolation is preserved through the wrapper rather than bypassed by it)
- [x] [input-validation] Input validation (Unknown, known-but-unimplemented and duplicate grant names each fail boot with a distinct named error; an unknown schema version fails and a missing one warns; a bare-string script entry is refused)
- [x] [thread-safety] Thread safety (Capabilities are resolved at boot and travel with the bytecode, so every pooled state re-running the chunks installs the same set; suite passes under -race)
- [x] [configurability] Configurability (Grants are per script in config.json with an explicit schemaVersion, rather than a server-wide switch)

## QA

Verified independently: build, vet and the full suite green under -race with both 052 and 058 in the tree. Confirmed capability.go owns NewEngineWithGrants, ungranted, loadChunk and grantedCapability, and that engine.go cleanly contains both stories' changes. 6/6 criteria, both new behaviours mutation-checked.

## Work Log

### 2026-09-13T18:26:50.959Z - Implemented per-script capability injection in internal/services/lua/capability.go. Capabilities are object-capabilities: a granted script gets its own copy of the indri host table with its capability tables raw-set on it, frozen, and bound to every handler it registered via a wrapper that seeds the per-invocation environment; an ungranted script keeps the shared table unchanged, so a capability it was not granted is absent from indri, from _G, from package.loaded and from any reachable metatable rather than being present behind a permission check. config.json now carries schemaVersion (models.ScriptSchemaVersion = 1; an unknown version fails boot, a missing one warns) and its scripts entries are {path, grants} objects. Unknown, unimplemented and duplicated grant names all fail boot with named errors; http and assets are registered as known names with no installer yet (story 059). An empty or missing scripts list warns loudly in script.NewStore. Tests: capability_test.go (injection, absence, per-script isolation across two scripts in one engine, read-only scoped tables, per-invocation globals, boot refusals) and script_test.go (schema version, warnings, grants carried through path resolution). Both new behaviours were mutation-checked. go build, go vet and go test -race pass across the repo.


### 2026-09-13T18:27:47.653Z - Proof completeness set PROVEN: 6 of 6 criteria; injection, absence, per-script isolation, read-only scoped tables and three distinct boot refusals all covered without a database

### 2026-09-13T18:27:47.728Z - Proof feature-availability set PROVEN: Boot resolves grants from config.json through NewEngineWithGrants; http and assets are registered names whose installers arrive in story 059

### 2026-09-13T18:27:47.798Z - Proof robustness set PROVEN: scopedHandler rebinds every handler a granted chunk registered, because pooledState.call installs a fresh environment falling back to the shared globals - without it a capability works at load time and is nil the moment a player triggers the handler

### 2026-09-13T18:27:47.873Z - Proof resilience set PROVEN: An ungranted script keeps the shared table and gets no wrapper, so the pre-existing load path is unchanged and a capability fault cannot affect scripts that were granted nothing

### 2026-09-13T18:27:47.951Z - Proof security set PROVEN: An ungranted capability is absent from indri, _G, package.loaded, package.preload and every reachable metatable rather than present behind a permission check; the walk is a test and leaking the capability onto the shared table was mutation-checked to fail

### 2026-09-13T18:27:48.029Z - Proof defense-in-depth set PROVEN: Scoped tables are frozen parent and children, and per-invocation global isolation is preserved through the wrapper rather than bypassed by it

### 2026-09-13T18:27:48.108Z - Proof input-validation set PROVEN: Unknown, known-but-unimplemented and duplicate grant names each fail boot with a distinct named error; an unknown schema version fails and a missing one warns; a bare-string script entry is refused

### 2026-09-13T18:27:48.186Z - Proof thread-safety set PROVEN: Capabilities are resolved at boot and travel with the bytecode, so every pooled state re-running the chunks installs the same set; suite passes under -race

### 2026-09-13T18:27:48.265Z - Proof configurability set PROVEN: Grants are per script in config.json with an explicit schemaVersion, rather than a server-wide switch
