---
id: 058-6453
title: Per-script capability injection
status: ready
priority: P2
type: feature
created: "2026-09-12T01:28:12.573Z"
updated: "2026-09-13T18:12:46.241Z"
dependencies: ["051"]
plan: plans/lua-game-scripting.md
plan_step: Step 13
depends_on: ["stories/051-0e8c-pending-P2-script-handler-reachable-through-the-router.md"]
---

# Per-script capability injection

## Problem Statement

A shared namespace with internal permission checks makes auditing what a script can do a read of every function body. Absent means unreachable is the stronger property. config.json also has no validation of any kind today.

## Acceptance Criteria

- [ ] A script without a grant sees the capability table as nil rather than a permission check inside an always-present function
- [ ] Grants are per-script, so one script grant is invisible to another loaded in the same process
- [ ] An ungranted capability cannot be reached by walking the globals table or a shared metatable
- [ ] An unknown grant name fails boot with a named error rather than silently granting nothing
- [ ] config.json gains a schema version field so a later format change is detectable
- [ ] An empty or missing scripts list warns loudly at boot rather than silently serving no actions
- [ ] VERIFY: go test ./internal/services/lua/

## Files

- internal/services/lua/capability.go

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

