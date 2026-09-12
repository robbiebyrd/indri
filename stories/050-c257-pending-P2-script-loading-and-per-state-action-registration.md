---
id: 050-c257
title: Script loading and per-state action registration
status: pending
priority: P2
type: feature
created: "2026-09-12T01:27:37.167Z"
updated: "2026-09-12T01:29:07.420Z"
dependencies: ["047"]
plan: plans/lua-game-scripting.md
plan_step: Step 5
depends_on: ["stories/047-3b39-pending-P2-compile-cache-and-pooled-lua-states-with-per-invoc.md"]
---

# Script loading and per-state action registration

## Problem Statement

indri.on registers an anonymous closure that may capture upvalues. A FunctionProto cannot reconstruct upvalues and an LFunction is bound to the state that created it, so a single global handler map called on a pooled state is wrong.

## Acceptance Criteria

- [ ] models.Script gains a scripts list resolved relative to the config file, and script loading returns errors instead of calling log.Fatalf
- [ ] Each pooled state runs every chunk itself and owns its own handler map
- [ ] The engine holds only the compiled protos and a frozen action manifest collected once at boot
- [ ] Registering the same action twice fails at load, including across two different script files
- [ ] A syntax error names the file and the line
- [ ] The action names received and processed, plus every built-in action name, are reserved and refused at load
- [ ] Boot fails loudly if two states disagree about which actions exist
- [ ] VERIFY: go test ./internal/services/lua/

## Files

- internal/services/lua/engine.go
- internal/repo/script/script.go
- internal/models/config.go

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

