---
id: 054-5cca
title: indri.reply and indri.send with depth and fan-out budgets
status: pending
priority: P2
type: feature
created: "2026-09-12T01:28:12.569Z"
updated: "2026-09-12T01:29:07.796Z"
dependencies: ["053"]
plan: plans/lua-game-scripting.md
plan_step: Step 9
depends_on: ["stories/053-7102-pending-P1-two-level-effect-ledger-for-script-side-effects.md"]
---

# indri.reply and indri.send with depth and fan-out budgets

## Problem Statement

Inline dispatch of a script-emitted event is the largest source of unbounded recursion in every event system surveyed. Separately, boot.handleClientMessage only logs a handler error, so a script error returned as a Go error would be invisible to WebSocket clients while REST and GraphQL callers see it.

## Acceptance Criteria

- [ ] reply lands in Result.Responses in order
- [ ] send dispatches only after the handler returns and never inline from within it
- [ ] A handler that sends its own action terminates at the depth cap with a script error
- [ ] Fan-out beyond the per-event cap errors
- [ ] Script errors reach the client as a models.WSError inside Result.Responses rather than as the Go error return
- [ ] The error response names the script and line for operator-authored scripts, with full detail logged server-side under a correlation id
- [ ] Error delivery is asserted on WebSocket, GraphQL and REST
- [ ] Calling reply twice either errors or the per-transport divergence is documented, since REST and GraphQL surface only the first response
- [ ] VERIFY: go test ./internal/services/lua/

## Files

- internal/services/lua/host_io.go

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

