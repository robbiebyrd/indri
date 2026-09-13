---
id: 055-c8e1
title: Shared helper library in Lua served from memory
status: complete
priority: P2
type: feature
created: "2026-09-12T01:28:12.570Z"
updated: "2026-09-13T18:03:47.297Z"
dependencies: ["050"]
plan: plans/lua-game-scripting.md
plan_step: Step 10
depends_on: ["stories/050-c257-pending-P2-script-loading-and-per-state-action-registration.md"]
completed_at: "2026-09-13T18:03:47.297Z"
---

# Shared helper library in Lua served from memory

## Problem Statement

Game helpers such as current scene, team of player and leader by score need no Go. Writing them in Lua makes them testable, shareable and overridable, and they become the seed of the shared library system.

## Acceptance Criteria

- [x] current_scene, scene_data, team_of, players_in_team, leader_by_score and each_team are implemented in Lua and embedded with go:embed
- [x] leader_by_score has a defined and tested behaviour for ties
- [x] Modules are served through PreloadModule from an in-memory registry
- [x] require of any filesystem path or relative path fails
- [x] Go tests drive each helper against fixture states
- [x] VERIFY: go test ./internal/services/lua/...

## Files

- internal/services/lua/lib/indri/game.lua
- internal/services/lua/lib/loader.go

## Proof

- [x] [completeness] Completeness (6 of 6 criteria; helpers written in Lua, driven by Go tests over fixtures built from models.Game through encoding/json so a renamed json tag breaks them)
- [x] [feature-availability] Feature availability (require of the shared module resolves from a real script chunk loaded through NewEngine, and still resolves after freezeState)
- [x] [robustness] Robustness (Every helper substitutes an empty table for an absent one, and every map walk goes through a sorted key list so no answer depends on hash order)
- [~] [resilience] Resilience (Pure Lua helpers over caller-supplied state; no I/O and no external dependency)
- [x] [security] Security (Eight module name forms including relative, absolute and the module's real on-disk path are all rejected; the test was mutation-checked so it is not vacuously passing)
- [x] [defense-in-depth] Defense in depth (preload is the only reachable searcher because sandbox.go already empties package.path and cpath and nils loadlib)
- [x] [input-validation] Input validation (Absent tables are substituted rather than raising, so a game with no teams does not error)
- [x] [thread-safety] Thread safety (Each module compiles once for the process via sync.OnceValues and the preload entry instantiates the shared proto on the requiring state, so no LFunction crosses a state boundary)
- [~] [configurability] Configurability (Fixed module set embedded at build time; per-user permissioned libraries are deferred to the admin UI)

## QA

Verified independently: build, vet, suite green under -race; no unexpected skips. Tie-break pinned by a test that rebuilds the players table in both insertion orders, because gopher-lua walks hash parts in insertion order and a naive tie test passes by accident. 6/6.

## Work Log

### 2026-09-13T18:02:25.714Z - Added internal/services/lua/lib: indri/game.lua (current_scene, scene_data, team_of, players_in_team, leader_by_score, each_team) embedded with go:embed, and loader.go which compiles each module once per process, serves it through L.PreloadModule and requires it once while the state is still writable, so the freeze cannot lock it out. lib.Install is called at the end of sandbox.newState, before the pool freezes; the pool's prepare hook is untouched. leader_by_score breaks ties on the lowest player id compared as a string, documented in the module and pinned by tests that also vary table insertion order. Tests: lib/lib_test.go drives every helper table-driven against models.Game fixtures marshalled through encoding/json, and uses NewEngine to prove require("indri.game") works from a script chunk while eight filesystem, relative and absolute module names all fail; lua/preload_test.go proves require still resolves after freezeState and that an unwarmed module does not. go build, go vet and go test -race all green, no skips.


### 2026-09-13T18:03:46.505Z - Proof completeness set PROVEN: 6 of 6 criteria; helpers written in Lua, driven by Go tests over fixtures built from models.Game through encoding/json so a renamed json tag breaks them

### 2026-09-13T18:03:46.581Z - Proof feature-availability set PROVEN: require of the shared module resolves from a real script chunk loaded through NewEngine, and still resolves after freezeState

### 2026-09-13T18:03:46.656Z - Proof robustness set PROVEN: Every helper substitutes an empty table for an absent one, and every map walk goes through a sorted key list so no answer depends on hash order

### 2026-09-13T18:03:46.734Z - Proof resilience set NOT_APPLICABLE: Pure Lua helpers over caller-supplied state; no I/O and no external dependency

### 2026-09-13T18:03:46.815Z - Proof security set PROVEN: Eight module name forms including relative, absolute and the module's real on-disk path are all rejected; the test was mutation-checked so it is not vacuously passing

### 2026-09-13T18:03:46.898Z - Proof defense-in-depth set PROVEN: preload is the only reachable searcher because sandbox.go already empties package.path and cpath and nils loadlib

### 2026-09-13T18:03:46.982Z - Proof input-validation set PROVEN: Absent tables are substituted rather than raising, so a game with no teams does not error

### 2026-09-13T18:03:47.062Z - Proof thread-safety set PROVEN: Each module compiles once for the process via sync.OnceValues and the preload entry instantiates the shared proto on the requiring state, so no LFunction crosses a state boundary

### 2026-09-13T18:03:47.144Z - Proof configurability set NOT_APPLICABLE: Fixed module set embedded at build time; per-user permissioned libraries are deferred to the admin UI
