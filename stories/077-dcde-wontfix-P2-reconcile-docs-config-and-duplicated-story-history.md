---
id: 077-dcde
title: Reconcile docs, config and duplicated story history
status: wontfix
priority: P2
type: chore
created: "2026-10-01T13:29:44.862Z"
updated: "2026-10-01T13:51:34.357Z"
dependencies: ["076-4402"]
plan: plans/main-divergence-integration.md
plan_step: Step 10
depends_on: ["stories/076-4402-pending-P1-decide-the-client-layout-engine-and-board-renderer.md"]
completed_at: "2026-10-01T13:51:34.357Z"
---

# Reconcile docs, config and duplicated story history

## Problem Statement

The merge conflicts include CLAUDE.md, AGENTS.md, README.md, both docs/ reference files, .env.example, the CI workflow, both config.json files, and 11 archived stories that were completed twice by different commits on each side. Left unreconciled, CLAUDE.md would describe neither the integrated system nor either side's actual state.

## Acceptance Criteria

- [ ] CLAUDE.md describes the integrated architecture: four DB backends, the slot model, the compact delta protocol and the Lua engine
- [ ] docs/ARCHITECTURE.md and docs/PROTOCOL.md match the transport stack actually chosen
- [ ] The 11 duplicated stories/archive entries are deduplicated, with the duplicate delivery noted in a worklog
- [ ] .env.example documents every INDRI_ variable the integrated build reads
- [ ] The CI workflow runs the integrated test suite including the Postgres integration tests
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

### 2026-10-01T13:51:34.294Z - Superseded by the trunk reversal: origin/main lacks the connection-independent dispatch refactor, so local main became the trunk and the port direction inverted. Plan plans/main-divergence-integration.md revised; this story's criteria were written for the opposite direction. Replaced by the revised story set.

