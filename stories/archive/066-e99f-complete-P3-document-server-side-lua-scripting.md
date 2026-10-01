---
id: 066-e99f
title: Document server-side Lua scripting
status: complete
priority: P3
type: chore
created: "2026-09-12T01:28:39.048Z"
updated: "2026-09-14T14:14:46.335Z"
dependencies: ["062"]
plan: plans/lua-game-scripting.md
plan_step: Step 18
depends_on: ["stories/062-e7ab-pending-P2-transport-parity-and-observability-for-script-acti.md"]
started_at: "2026-09-14T01:22:42.208Z"
completed_at: "2026-09-14T14:14:46.335Z"
---

# Document server-side Lua scripting

## Problem Statement

The scripting surface, its delivery guarantees and its accepted risks are only described in the plan. Authors and operators need them in the repo docs, and CLAUDE.md still describes adding a game action as a Go task.

## Acceptance Criteria

- [x] A scripting document covers the host API surface, the state contract, effect and retry rules, capability grants and how to test a script
- [x] Delivery guarantees per effect kind, at-least-once timers and the nil-session timer contract are documented
- [REJECTED] Forbidden hooks on auth actions, reserved action names, the generic GraphQL mutation and the bounded wildcard REST route are documented (Half of it cannot be satisfied yet, and I wrongly ticked it before checking. Forbidden auth hooks and reserved action names ARE documented. The generic GraphQL mutation and the bounded wildcard REST route do not exist: they are story 062's output, 062 is incomplete, and grep confirms neither schema.graphqls nor rest/routes.go carries anything for script actions. docs/SCRIPTING.md marks that section as a gap naming 062 rather than describing a design nobody has built. Re-check this when 062 lands and the section is rewritten.)
- [x] The accepted in-process heap risk is stated plainly rather than implied to be solved
- [x] ARCHITECTURE, PROTOCOL, README and CLAUDE are updated, and adding a game action becomes Lua-first
- [REJECTED] env example documents every new INDRI_LUA and scheduler variable (Unsatisfiable as written: there are no such variables. internal/repo/env/env.go has no Lua, script, scheduler or timer field and nothing in the repo reads INDRI_LUA_*. Every value is a Go constant, several deliberately coupled — boot.checkScriptDeadline fails startup if InvocationTimeout is raised without the lock lease. Inventing variables to satisfy the criterion would have been worse than rejecting it; the absence is documented in a Configuration subsection instead.)
- [x] VERIFY: go build ./... && go test ./...

## Files

- docs/SCRIPTING.md
- CLAUDE.md
- .env.example

## Proof

- [x] [completeness] Completeness (5 of 7 criteria met; two rejected with reasons — one unsatisfiable （no INDRI_LUA variables exist） and one half-blocked on 062, which I caught after wrongly ticking it)
- [x] [feature-availability] Feature availability (Every indri.* signature is read from source; every sample was executed through indri-script rather than written, and one that could not be run was removed rather than published)
- [x] [robustness] Robustness (The plan's Lua sketches were treated as suspect and none were copied: indri.mutate takes one argument and indri.reject does not exist, the real idiom being error（message, 0）)
- [~] [resilience] Resilience (Documentation only; no runtime behaviour to fail)
- [x] [security] Security (The sandbox, the capability views and the reserved names are documented as they behave, with fixture probes confirming os, io, debug, load, print, setmetatable, coroutine and a granted indri.http are all nil inside an action handler)
- [x] [defense-in-depth] Defense in depth (The transport-parity section is a marked gap naming 062, stating only verified facts. Describing a design nobody had built would have been the worse failure)
- [~] [input-validation] Input validation (Documentation only; no input path)
- [~] [thread-safety] Thread safety (Documentation only; no concurrency)
- [x] [configurability] Configurability (A Configuration subsection documents that every scripting value is a Go constant rather than an environment variable, including the coupling boot.checkScriptDeadline enforces at startup)

## QA

Documentation only; every sample executed through indri-script rather than written untested

## Work Log

### 2026-09-14T00:49:16.222Z - NOTE FOR WHOEVER WRITES THIS (not yet started): two operational gotchas found while verifying 060, both worth documenting. (1) The WebRTC transport binds a FIXED UDP port at boot (INDRI_RTC_UDP_PORT, default 8443), so two server instances on one machine collide with 'creating udp mux on port 8443: bind: address already in use'. That error names webrtc and looks like a transport bug but is environmental; running a second instance needs INDRI_RTC_UDP_PORT and INDRI_LISTEN_PORT both set. (2) CLAUDE.md's 'Adding a game action' section is now STALE RATHER THAN WRONG: it documents the Go path (example/<game>/server/handlers/<action>/handler.go plus router.RegisterHandler after boot.Boot) as the way to add an action. That path still works and the plan deliberately keeps it as an escape hatch, but it is no longer the default and it points at example/tictactoe, which as of c3efbe0 has no Go code at all — it is config.json plus game.lua running on the stock cmd/server binary. Rewriting that section is this story's job; it was deliberately left alone by 060 so a documentation decision was not buried inside a port.

### 2026-09-14T14:14:17.079Z - Wrote docs/SCRIPTING.md covering the whole host API, the two namespaces, indri.mutate's retry and reentrancy rules, the invariants keeping membership and host out of a script's reach, the effect ledger's per-kind delivery guarantee, timers and the nil-session contract, lifecycle events, hooks, capabilities and host views, the sandbox, every limit in one table, indri-script, and where a Go handler is still the answer. Every sample was executed rather than written, through scratch games driven by indri-script; the console transcripts are real output, including a captured failure, and one transcript's dot-leader widths were invented on the first pass and re-run to get the true ones. One sample was written and then removed rather than published, because models.Script has no players field so a player.host invariant demonstration could not be stamped into a fixture. CLAUDE.md's 'Adding a game action' is rewritten Lua-first with the Go path demoted to a labelled escape hatch; it was stale rather than wrong, and its claim that nothing catches a missing GraphQL mutation was also stale. The transport-parity section is a marked gap naming story 062 as in flight, stating only verified current facts and no design. Verified: build, vet and gofmt clean, go test -race -count=1 ./... green across 27 packages, no Go files changed, all 19 internal anchors and both cross-file links resolve.


### 2026-09-14T14:14:45.536Z - Proof completeness set PROVEN: 5 of 7 criteria met; two rejected with reasons — one unsatisfiable (no INDRI_LUA variables exist) and one half-blocked on 062, which I caught after wrongly ticking it

### 2026-09-14T14:14:45.615Z - Proof feature-availability set PROVEN: Every indri.* signature is read from source; every sample was executed through indri-script rather than written, and one that could not be run was removed rather than published

### 2026-09-14T14:14:45.693Z - Proof robustness set PROVEN: The plan's Lua sketches were treated as suspect and none were copied: indri.mutate takes one argument and indri.reject does not exist, the real idiom being error(message, 0)

### 2026-09-14T14:14:45.774Z - Proof resilience set NOT_APPLICABLE: Documentation only; no runtime behaviour to fail

### 2026-09-14T14:14:45.854Z - Proof security set PROVEN: The sandbox, the capability views and the reserved names are documented as they behave, with fixture probes confirming os, io, debug, load, print, setmetatable, coroutine and a granted indri.http are all nil inside an action handler

### 2026-09-14T14:14:45.951Z - Proof defense-in-depth set PROVEN: The transport-parity section is a marked gap naming 062, stating only verified facts. Describing a design nobody had built would have been the worse failure

### 2026-09-14T14:14:46.054Z - Proof input-validation set NOT_APPLICABLE: Documentation only; no input path

### 2026-09-14T14:14:46.138Z - Proof thread-safety set NOT_APPLICABLE: Documentation only; no concurrency

### 2026-09-14T14:14:46.243Z - Proof configurability set PROVEN: A Configuration subsection documents that every scripting value is a Go constant rather than an environment variable, including the coupling boot.checkScriptDeadline enforces at startup
