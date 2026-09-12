---
id: 055-c8e1
title: Shared helper library in Lua served from memory
status: pending
priority: P2
type: feature
created: "2026-09-12T01:28:12.570Z"
updated: "2026-09-12T01:29:07.868Z"
dependencies: ["050"]
plan: plans/lua-game-scripting.md
plan_step: Step 10
depends_on: ["stories/050-c257-pending-P2-script-loading-and-per-state-action-registration.md"]
---

# Shared helper library in Lua served from memory

## Problem Statement

Game helpers such as current scene, team of player and leader by score need no Go. Writing them in Lua makes them testable, shareable and overridable, and they become the seed of the shared library system.

## Acceptance Criteria

- [ ] current_scene, scene_data, team_of, players_in_team, leader_by_score and each_team are implemented in Lua and embedded with go:embed
- [ ] leader_by_score has a defined and tested behaviour for ties
- [ ] Modules are served through PreloadModule from an in-memory registry
- [ ] require of any filesystem path or relative path fails
- [ ] Go tests drive each helper against fixture states
- [ ] VERIFY: go test ./internal/services/lua/...

## Files

- internal/services/lua/lib/indri/game.lua
- internal/services/lua/lib/loader.go

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

