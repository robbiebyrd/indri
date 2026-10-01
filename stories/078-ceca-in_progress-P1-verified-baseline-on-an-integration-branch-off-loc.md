---
id: 078-ceca
title: Verified baseline on an integration branch off local main
status: in_progress
priority: P1
type: chore
created: "2026-10-01T13:52:29.867Z"
updated: "2026-10-01T14:02:08.561Z"
dependencies: []
plan: plans/main-divergence-integration.md
plan_step: Step 1
started_at: "2026-10-01T14:02:08.560Z"
---

# Verified baseline on an integration branch off local main

## Problem Statement

Local main is the trunk after the dispatch-model reversal. Every later port is judged against this baseline, so an unverified starting point makes it impossible to tell an integration regression from a pre-existing break. The working tree also carries an uncommitted story move that must be settled before branching.

## Acceptance Criteria

- [ ] The uncommitted stories/066-* move (deleted from stories/, re-added under stories/archive/) is settled and committed
- [ ] Branch POC-00003/main-integration exists, created from local main
- [ ] VERIFY: go build ./... && go vet ./...
- [ ] go test -race ./... output captured to .integration/baseline.txt and committed to the branch
- [ ] Client verified the way CI does: rm -rf node_modules && pnpm install --frozen-lockfile && pnpm run typecheck && pnpm test
- [ ] Any pre-existing failure is recorded in the baseline file with a note, not silently fixed

## Files

- .integration/baseline.txt

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

