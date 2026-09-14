---
id: 066-e99f
title: Document server-side Lua scripting
status: in_progress
priority: P3
type: chore
created: "2026-09-12T01:28:39.048Z"
updated: "2026-09-14T01:22:42.209Z"
dependencies: ["062"]
plan: plans/lua-game-scripting.md
plan_step: Step 18
depends_on: ["stories/062-e7ab-pending-P2-transport-parity-and-observability-for-script-acti.md"]
started_at: "2026-09-14T01:22:42.208Z"
---

# Document server-side Lua scripting

## Problem Statement

The scripting surface, its delivery guarantees and its accepted risks are only described in the plan. Authors and operators need them in the repo docs, and CLAUDE.md still describes adding a game action as a Go task.

## Acceptance Criteria

- [ ] A scripting document covers the host API surface, the state contract, effect and retry rules, capability grants and how to test a script
- [ ] Delivery guarantees per effect kind, at-least-once timers and the nil-session timer contract are documented
- [ ] Forbidden hooks on auth actions, reserved action names, the generic GraphQL mutation and the bounded wildcard REST route are documented
- [ ] The accepted in-process heap risk is stated plainly rather than implied to be solved
- [ ] ARCHITECTURE, PROTOCOL, README and CLAUDE are updated, and adding a game action becomes Lua-first
- [ ] env example documents every new INDRI_LUA and scheduler variable
- [ ] VERIFY: go build ./... && go test ./...

## Files

- docs/SCRIPTING.md
- CLAUDE.md
- .env.example

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

### 2026-09-14T00:49:16.222Z - NOTE FOR WHOEVER WRITES THIS (not yet started): two operational gotchas found while verifying 060, both worth documenting. (1) The WebRTC transport binds a FIXED UDP port at boot (INDRI_RTC_UDP_PORT, default 8443), so two server instances on one machine collide with 'creating udp mux on port 8443: bind: address already in use'. That error names webrtc and looks like a transport bug but is environmental; running a second instance needs INDRI_RTC_UDP_PORT and INDRI_LISTEN_PORT both set. (2) CLAUDE.md's 'Adding a game action' section is now STALE RATHER THAN WRONG: it documents the Go path (example/<game>/server/handlers/<action>/handler.go plus router.RegisterHandler after boot.Boot) as the way to add an action. That path still works and the plan deliberately keeps it as an escape hatch, but it is no longer the default and it points at example/tictactoe, which as of c3efbe0 has no Go code at all — it is config.json plus game.lua running on the stock cmd/server binary. Rewriting that section is this story's job; it was deliberately left alone by 060 so a documentation decision was not buried inside a port.

