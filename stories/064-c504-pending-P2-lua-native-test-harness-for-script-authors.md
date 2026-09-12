---
id: 064-c504
title: Lua-native test harness for script authors
status: pending
priority: P2
type: feature
created: "2026-09-12T01:28:39.045Z"
updated: "2026-09-12T01:29:08.785Z"
dependencies: ["060"]
plan: plans/lua-game-scripting.md
plan_step: Step 20
depends_on: ["stories/060-905f-pending-P2-port-tic-tac-toe-to-lua-and-delete-its-go-handlers.md"]
---

# Lua-native test harness for script authors

## Problem Statement

The premise of this plan is that a game author writes no Go, yet every other step validates with go test. Without an author-facing harness that premise is false.

## Acceptance Criteria

- [ ] A CLI runs a fixture directory of state, event and expected files with no database
- [ ] A readable diff is printed on mismatch and a failing fixture exits non-zero
- [ ] A check subcommand validates compile, manifest and grants without booting a server
- [ ] An author can verify a script without writing Go or knowing the Go toolchain
- [ ] The harness uses the in-memory store so no database is required
- [ ] VERIFY: go vet ./... && go test ./cmd/indri-script/

## Files

- cmd/indri-script/main.go

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

