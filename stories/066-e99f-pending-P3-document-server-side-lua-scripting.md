---
id: 066-e99f
title: Document server-side Lua scripting
status: pending
priority: P3
type: chore
created: "2026-09-12T01:28:39.048Z"
updated: "2026-09-12T01:29:08.931Z"
dependencies: ["062"]
plan: plans/lua-game-scripting.md
plan_step: Step 18
depends_on: ["stories/062-e7ab-pending-P2-transport-parity-and-observability-for-script-acti.md"]
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

