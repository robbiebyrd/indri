---
id: 061-3ade
title: Lua hooks on built-in actions
status: in_progress
priority: P2
type: feature
created: "2026-09-12T01:28:39.039Z"
updated: "2026-09-14T00:48:25.587Z"
dependencies: ["060", "045"]
plan: plans/lua-game-scripting.md
plan_step: Step 16
depends_on: ["stories/060-905f-pending-P2-port-tic-tac-toe-to-lua-and-delete-its-go-handlers.md", "stories/045-795e-pending-P1-thread-context-and-action-name-through-dispatch-mu.md"]
started_at: "2026-09-14T00:48:25.586Z"
---

# Lua hooks on built-in actions

## Problem Statement

Built-in actions stay in Go, so scripts extend them through the router received and processed phases. But received runs for every message including pre-auth login, whose payload carries a plaintext password, and Dispatch returns early on error so processed is not an unconditional after phase.

## Acceptance Criteria

- [ ] before and after-success hooks fire around the Go handler for a given action
- [ ] Hooks on login, register, reconnect and logout are refused at load time
- [ ] The payload is stripped for any hook firing on an unauthenticated request
- [ ] The two hook kinds are named honestly as before and after-success, since a failed action never reaches the processed phase
- [ ] Hooks on an unknown action name are refused at load
- [ ] The no-hook path adds no measurable latency, because received runs for every inbound message
- [ ] Before-hooks are documented as validation-only, since a before-hook mutation commits independently of an action that later fails
- [ ] VERIFY: go vet ./... && go test ./...

## Files

- internal/services/lua/hooks.go

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

