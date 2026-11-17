---
id: 068-223a
title: Establish a verified trunk baseline on the integration branch
status: wontfix
priority: P1
type: chore
created: "2026-10-01T13:29:44.839Z"
updated: "2026-10-01T13:51:32.718Z"
dependencies: []
plan: plans/main-divergence-integration.md
plan_step: Step 1
completed_at: "2026-10-01T13:51:32.718Z"
---

# Establish a verified trunk baseline on the integration branch

## Problem Statement

origin/main carries 11 merge commits from parallel wip/* branches and nobody has confirmed it builds or passes tests. Every later integration step is judged against this baseline, so replaying local work onto an unverified trunk would make it impossible to tell an integration regression from a pre-existing break.

## Acceptance Criteria

- [ ] Branch POC-00003/main-integration exists, created from origin/main
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

### 2026-10-01T13:51:32.653Z - Superseded by the trunk reversal: origin/main lacks the connection-independent dispatch refactor, so local main became the trunk and the port direction inverted. Plan plans/main-divergence-integration.md revised; this story's criteria were written for the opposite direction. Replaced by the revised story set.

