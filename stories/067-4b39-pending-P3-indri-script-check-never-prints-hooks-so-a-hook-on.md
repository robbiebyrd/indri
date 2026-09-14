---
id: "067-4b39"
title: "indri-script check never prints hooks, so a hook-only script reports as declaring nothing"
status: pending
priority: P3
type: fix
created: 2026-09-14T14:14:57.778Z
updated: 2026-09-14T14:14:57.778Z
dependencies: []
---

# indri-script check never prints hooks, so a hook-only script reports as declaring nothing

## Problem Statement

internal/services/scripttest/check.go:56 prints scripts, actions and lifecycle events only. Engine.Hooks() exists and is a settled manifest held to the same agreement rule as the other two, but check never reads it. A script whose entire contribution is indri.before or indri.after_action therefore reports as declaring nothing at all, which reads as a script that failed to load rather than one that loaded and registered hooks. Found while writing docs/SCRIPTING.md and reported rather than fixed, because scripttest was adjacent to a do-not-touch fence at the time and this is a behaviour change rather than a doc correction. Documented in SCRIPTING.md under what the harness does not cover.

## Acceptance Criteria

- [ ] check prints the hooks a script registered, alongside its actions and lifecycle events
- [ ] A script declaring only hooks reports them rather than appearing to declare nothing
- [ ] A test asserts hook output, and fails if Hooks() is ignored
- [ ] docs/SCRIPTING.md stops listing this under what the harness does not cover
- [ ] VERIFY: go test -race ./internal/services/scripttest/

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

