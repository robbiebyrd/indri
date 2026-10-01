---
id: 078-ceca
title: Verified baseline on an integration branch off local main
status: complete
priority: P1
type: chore
created: "2026-10-01T13:52:29.867Z"
updated: "2026-10-01T14:07:31.079Z"
dependencies: []
plan: plans/main-divergence-integration.md
plan_step: Step 1
started_at: "2026-10-01T14:02:08.560Z"
completed_at: "2026-10-01T14:07:31.079Z"
---

# Verified baseline on an integration branch off local main

## Problem Statement

Local main is the trunk after the dispatch-model reversal. Every later port is judged against this baseline, so an unverified starting point makes it impossible to tell an integration regression from a pre-existing break. The working tree also carries an uncommitted story move that must be settled before branching.

## Acceptance Criteria

- [x] The uncommitted stories/066-* move (deleted from stories/, re-added under stories/archive/) is settled and committed
- [x] Branch POC-00003/main-integration exists, created from local main
- [x] VERIFY: go build ./... && go vet ./...
- [x] go test -race ./... output captured to .integration/baseline.txt and committed to the branch
- [x] Client verified the way CI does: rm -rf node_modules && pnpm install --frozen-lockfile && pnpm run typecheck && pnpm test
- [x] Any pre-existing failure is recorded in the baseline file with a note, not silently fixed

## Files

- .integration/baseline.txt

## Proof

- [x] [completeness] Completeness (All 6 criteria verified with captured output in .integration/baseline.txt: build, vet, race suite, clean client install, typecheck, client tests.)
- [~] [feature-availability] Feature availability (This story changes no production code. It creates a branch and records a measured baseline; there is no new surface, behaviour or configuration to assess.)
- [~] [robustness] Robustness (This story changes no production code. It creates a branch and records a measured baseline; there is no new surface, behaviour or configuration to assess.)
- [~] [resilience] Resilience (This story changes no production code. It creates a branch and records a measured baseline; there is no new surface, behaviour or configuration to assess.)
- [~] [security] Security (This story changes no production code. It creates a branch and records a measured baseline; there is no new surface, behaviour or configuration to assess.)
- [~] [defense-in-depth] Defense in depth (This story changes no production code. It creates a branch and records a measured baseline; there is no new surface, behaviour or configuration to assess.)
- [~] [input-validation] Input validation (This story changes no production code. It creates a branch and records a measured baseline; there is no new surface, behaviour or configuration to assess.)
- [x] [thread-safety] Thread safety (go test -race ./... exit 0 across 27 packages with zero DATA RACE reports; grep for 'DATA RACE|panic:' in baseline.txt returns none.)
- [~] [configurability] Configurability (This story changes no production code. It creates a branch and records a measured baseline; there is no new surface, behaviour or configuration to assess.)

## QA

Each command run and full output captured to .integration/baseline.txt: go build exit 0; go vet exit 0; go test -race exit 0 with 27 packages ok, 0 FAIL, no DATA RACE; client node_modules deleted then pnpm install --frozen-lockfile exit 0; pnpm run typecheck exit 0; pnpm test 413 pass 0 fail.

## Work Log

### 2026-10-01T14:07:17.900Z - Baseline established on POC-00003/main-integration at cff6b5f. go build PASS, go vet PASS, go test -race PASS (27 packages ok, 0 FAIL, no data races). Client clean --frozen-lockfile install PASS, typecheck PASS, tests 413/413. No pre-existing failures to carry. Criterion 6 satisfied vacuously: nothing was failing, so nothing was recorded or fixed.


### 2026-10-01T14:07:28.476Z - Proof completeness set PROVEN: All 6 criteria verified with captured output in .integration/baseline.txt: build, vet, race suite, clean client install, typecheck, client tests.

### 2026-10-01T14:07:28.551Z - Proof thread-safety set PROVEN: go test -race ./... exit 0 across 27 packages with zero DATA RACE reports; grep for 'DATA RACE|panic:' in baseline.txt returns none.

### 2026-10-01T14:07:28.623Z - Proof feature-availability set NOT_APPLICABLE: This story changes no production code. It creates a branch and records a measured baseline; there is no new surface, behaviour or configuration to assess.

### 2026-10-01T14:07:28.693Z - Proof robustness set NOT_APPLICABLE: This story changes no production code. It creates a branch and records a measured baseline; there is no new surface, behaviour or configuration to assess.

### 2026-10-01T14:07:28.770Z - Proof resilience set NOT_APPLICABLE: This story changes no production code. It creates a branch and records a measured baseline; there is no new surface, behaviour or configuration to assess.

### 2026-10-01T14:07:28.852Z - Proof security set NOT_APPLICABLE: This story changes no production code. It creates a branch and records a measured baseline; there is no new surface, behaviour or configuration to assess.

### 2026-10-01T14:07:28.936Z - Proof defense-in-depth set NOT_APPLICABLE: This story changes no production code. It creates a branch and records a measured baseline; there is no new surface, behaviour or configuration to assess.

### 2026-10-01T14:07:29.017Z - Proof input-validation set NOT_APPLICABLE: This story changes no production code. It creates a branch and records a measured baseline; there is no new surface, behaviour or configuration to assess.

### 2026-10-01T14:07:29.099Z - Proof configurability set NOT_APPLICABLE: This story changes no production code. It creates a branch and records a measured baseline; there is no new surface, behaviour or configuration to assess.
