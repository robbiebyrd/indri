---
id: 087-c23c
title: Reconcile docs, config and duplicated story history
status: pending
priority: P2
type: chore
created: "2026-10-01T13:52:35.319Z"
updated: "2026-10-01T13:52:40.163Z"
dependencies: ["086-59e8"]
plan: plans/main-divergence-integration.md
plan_step: Step 10
depends_on: ["stories/086-59e8-pending-P1-settle-the-origin-policy-and-harvest-the-remaining.md"]
---

# Reconcile docs, config and duplicated story history

## Problem Statement

Eleven archived stories were completed twice by different commits on each side, and the reference documentation describes neither the integrated system nor either side's current state. CLAUDE.md in particular is the file future sessions rely on to avoid repeating this divergence.

## Acceptance Criteria

- [ ] CLAUDE.md describes the integrated architecture: four storage backends, the slot model, and whichever delta format was chosen
- [ ] docs/ARCHITECTURE.md and docs/PROTOCOL.md match what actually shipped
- [ ] The 11 duplicated stories/archive entries are deduplicated, with the duplicate delivery noted in a worklog
- [ ] .env.example documents every INDRI_ variable the integrated build reads
- [ ] The CI workflow runs the integrated suite including the Postgres integration tests
- [ ] VERIFY: go build ./... && go vet ./... && go test -race ./...
- [ ] Full result compared against .integration/baseline.txt with no regression

## Files

- CLAUDE.md
- AGENTS.md
- README.md
- docs/ARCHITECTURE.md
- docs/PROTOCOL.md
- .env.example
- .github/workflows/ci.yml

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

