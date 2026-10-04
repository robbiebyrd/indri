---
id: "067-4b39"
title: "indri-script check never prints hooks, so a hook-only script reports as declaring nothing"
status: complete
priority: P3
type: fix
created: 2026-09-14T14:14:57.778Z
updated: 2026-10-04T18:53:31.000Z
dependencies: []
---

# indri-script check never prints hooks, so a hook-only script reports as declaring nothing

## Problem Statement

internal/services/scripttest/check.go:56 prints scripts, actions and lifecycle events only. Engine.Hooks() exists and is a settled manifest held to the same agreement rule as the other two, but check never reads it. A script whose entire contribution is indri.before or indri.after_action therefore reports as declaring nothing at all, which reads as a script that failed to load rather than one that loaded and registered hooks. Found while writing docs/SCRIPTING.md and reported rather than fixed, because scripttest was adjacent to a do-not-touch fence at the time and this is a behaviour change rather than a doc correction. Documented in SCRIPTING.md under what the harness does not cover.

## Acceptance Criteria

- [x] check prints the hooks a script registered, alongside its actions and lifecycle events
- [x] A script declaring only hooks reports them rather than appearing to declare nothing
- [x] A test asserts hook output, and fails if Hooks() is ignored
- [x] docs/SCRIPTING.md stops listing this under what the harness does not cover
- [x] VERIFY: go test -race ./internal/services/scripttest/

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

### 2026-10-04T18:53:31.000Z - FIX. Check now prints a hooks section after lifecycle events, one line per Engine.Hooks() entry as "<kind> <action>" in the manifest's own order (kind, then action). New fixture testdata/hooksonly declares only indri.before("join") and indri.after_action("leave"); TestCheck_ReportsHooks asserts it reports "actions (none)" and "hooks after-success leave / before join". The test failed first against the old check (no hooks section), then passed. SCRIPTING.md: the check bullet names hooks and the "does not list hooks" gap is removed. go test -race ./internal/services/scripttest/ green. Proof items: the change only prints a manifest the engine already validated; there is no new input, concurrency or configuration surface.

