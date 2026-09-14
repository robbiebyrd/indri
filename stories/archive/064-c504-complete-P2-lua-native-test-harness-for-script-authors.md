---
id: 064-c504
title: Lua-native test harness for script authors
status: complete
priority: P2
type: feature
created: "2026-09-12T01:28:39.045Z"
updated: "2026-09-14T01:14:11.090Z"
dependencies: ["060"]
plan: plans/lua-game-scripting.md
plan_step: Step 20
depends_on: ["stories/060-905f-pending-P2-port-tic-tac-toe-to-lua-and-delete-its-go-handlers.md"]
started_at: "2026-09-14T00:48:25.734Z"
completed_at: "2026-09-14T01:14:11.090Z"
---

# Lua-native test harness for script authors

## Problem Statement

The premise of this plan is that a game author writes no Go, yet every other step validates with go test. Without an author-facing harness that premise is false.

## Acceptance Criteria

- [x] A CLI runs a fixture directory of state, event and expected files with no database
- [x] A readable diff is printed on mismatch and a failing fixture exits non-zero
- [x] A check subcommand validates compile, manifest and grants without booting a server
- [x] An author can verify a script without writing Go or knowing the Go toolchain
- [x] The harness uses the in-memory store so no database is required
- [x] VERIFY: go vet ./... && go test ./cmd/indri-script/

## Files

- cmd/indri-script/main.go

## Proof

- [x] [completeness] Completeness (The harness runs a game's scripts with no server and no database, reproduces nine tic-tac-toe golden cases against the real config and game.lua, and agrees with the Go suite's verdicts)
- [x] [feature-availability] Feature availability (Driven end to end as a binary, not only as a library: 9/9 ok and exit 0 against the real fixtures, and exit 2 with a clear message when the config declares no scripts)
- [x] [robustness] Robustness (A failing case names the fixture, the case and the diff; a faulting script reports its file and line plus a correlation id. Mutants that blanked the check or dropped the script's message were all caught)
- [x] [resilience] Resilience (Exit 2 separates 'the harness never ran' from 'a case failed', so CI can tell a broken fixture set from a broken game. I found the one path that violated this and fixed it)
- [x] [security] Security (Real engine and real game.MemoryStore throughout, not a stub, so the harness cannot pass a script the server would refuse; timers write to a real in-memory schedule store rather than being refused)
- [x] [defense-in-depth] Defense in depth (A run with zero fixtures or zero cases is an error, not a silent pass: the mutant that accepted it literally printed '0 cases, all ok' and exited 0, and is caught by four tests)
- [x] [input-validation] Input validation (DisallowUnknownFields means a misspelled fixture key is a loud parse failure rather than a silently ignored expectation; a malformed fixture is exit 2)
- [x] [thread-safety] Thread safety (One engine and one store per run with cases isolated by playing their own game and deltas keyed by game id; go test -race green)
- [x] [configurability] Configurability (The config is selectable with -script and defaults to a named directory's own config.json; the timeout is a flag; -v lets the host's tracebacks through)

## QA

None — covered by tests in internal/services/scripttest, including a reproduction of nine tic-tac-toe golden cases

## Work Log

### 2026-09-14T01:14:09.585Z - Built cmd/indri-script plus internal/services/scripttest: a fixture harness running a game's Lua against fixture state through the real engine and a real game.MemoryStore, with no server and no database. Fixtures are JSON rather than a Lua assertion API for a technical reason, not taste: the delta is computed in Go by events.Diff after the indri.mutate callback returns and the versioned save commits, so a script inside the sandbox can only read state, which is strictly weaker, and 'the write happened but never published' is the failure this repo cares most about. One file holds many cases rather than the plan's three files per case, which would have been ~90 files for tic-tac-toe. Nine cases from example/tictactoe/game_test.go are reproduced against the real config and game.lua and agree with the Go suite. Exit codes separate 'your script is wrong' (1) from 'the harness never ran' (2). I found and fixed a defect in the agent's work during verification: running the harness against a fixture directory with no config.json fell back to the config.json in the working directory, which in this repo is the framework's own and declares no scripts, so all nine cases failed with 'no lua handler is registered' — nine failures reading as a broken game when nothing had been loaded. A config declaring no scripts is now refused up front as a harness failure naming the config, with a regression test I mutation-checked. Verified: gofmt clean, build and vet clean, go test -race -count=1 ./... green tree-wide, and I drove the binary end to end against the real tic-tac-toe fixtures (9/9 ok, exit 0). Known gaps, all reported rather than silent: lifecycle events are not dispatchable from a fixture, timers are recorded but never fired, and a script publishing two deltas cannot be expressed.


### 2026-09-14T01:14:10.348Z - Proof completeness set PROVEN: The harness runs a game's scripts with no server and no database, reproduces nine tic-tac-toe golden cases against the real config and game.lua, and agrees with the Go suite's verdicts

### 2026-09-14T01:14:10.427Z - Proof feature-availability set PROVEN: Driven end to end as a binary, not only as a library: 9/9 ok and exit 0 against the real fixtures, and exit 2 with a clear message when the config declares no scripts

### 2026-09-14T01:14:10.505Z - Proof robustness set PROVEN: A failing case names the fixture, the case and the diff; a faulting script reports its file and line plus a correlation id. Mutants that blanked the check or dropped the script's message were all caught

### 2026-09-14T01:14:10.575Z - Proof resilience set PROVEN: Exit 2 separates 'the harness never ran' from 'a case failed', so CI can tell a broken fixture set from a broken game. I found the one path that violated this and fixed it

### 2026-09-14T01:14:10.646Z - Proof security set PROVEN: Real engine and real game.MemoryStore throughout, not a stub, so the harness cannot pass a script the server would refuse; timers write to a real in-memory schedule store rather than being refused

### 2026-09-14T01:14:10.718Z - Proof defense-in-depth set PROVEN: A run with zero fixtures or zero cases is an error, not a silent pass: the mutant that accepted it literally printed '0 cases, all ok' and exited 0, and is caught by four tests

### 2026-09-14T01:14:10.791Z - Proof input-validation set PROVEN: DisallowUnknownFields means a misspelled fixture key is a loud parse failure rather than a silently ignored expectation; a malformed fixture is exit 2

### 2026-09-14T01:14:10.864Z - Proof thread-safety set PROVEN: One engine and one store per run with cases isolated by playing their own game and deltas keyed by game id; go test -race green

### 2026-09-14T01:14:10.938Z - Proof configurability set PROVEN: The config is selectable with -script and defaults to a named directory's own config.json; the timeout is a flag; -v lets the host's tracebacks through
