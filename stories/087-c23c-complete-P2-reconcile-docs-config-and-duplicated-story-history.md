---
id: 087-c23c
title: Reconcile docs, config and duplicated story history
status: complete
priority: P2
type: chore
created: "2026-10-01T13:52:35.319Z"
updated: "2026-10-04T18:49:29.000Z"
dependencies: ["086-59e8"]
plan: plans/main-divergence-integration.md
plan_step: Step 10
depends_on: ["stories/086-59e8-pending-P1-settle-the-origin-policy-and-harvest-the-remaining.md"]
---

# Reconcile docs, config and duplicated story history

## Problem Statement

Eleven archived stories were completed twice by different commits on each side, and the reference documentation describes neither the integrated system nor either side's current state. CLAUDE.md in particular is the file future sessions rely on to avoid repeating this divergence.

## Acceptance Criteria

- [x] CLAUDE.md describes the integrated architecture: four storage backends, the slot model, and whichever delta format was chosen
- [x] docs/ARCHITECTURE.md and docs/PROTOCOL.md match what actually shipped
- [x] The 11 duplicated stories/archive entries are deduplicated, with the duplicate delivery noted in a worklog
- [x] .env.example documents every INDRI_ variable the integrated build reads
- [x] The CI workflow runs the integrated suite including the Postgres integration tests
- [x] VERIFY: go build ./... && go vet ./... && go test -race ./...
- [x] Full result compared against .integration/baseline.txt with no regression

## Files

- CLAUDE.md
- AGENTS.md
- README.md
- docs/ARCHITECTURE.md
- docs/PROTOCOL.md
- .env.example
- .github/workflows/ci.yml

## Proof

- [x] [completeness] Completeness
- [x] [feature-availability] Feature availability
- [x] [robustness] Robustness
- [x] [resilience] Resilience
- [x] [security] Security
- [x] [defense-in-depth] Defense in depth
- [x] [input-validation] Input validation
- [x] [thread-safety] Thread safety
- [x] [configurability] Configurability

## Work Log

### 2026-10-04T18:49:29.000Z - DUPLICATES. The "11 duplicated stories" are 012-020, 023 and 024: in both main's archive (16 stories, all present here under the same ids) and this branch's, completed separately — here first (2026-09-10, with QA, proof and device confirmation), on main later (c07e901, 2026-09-16, web-only proof). This branch's copy of each is canonical; each now carries a DUPLICATE DELIVERY worklog entry naming main's commit and completion time. Separately noted, not duplicates: 11 numbers in this archive are shared by two or more distinct stories (067-072 and 078-082), all added on this branch's own lineage by parallel worktrees; their ids (number plus hash) are unique, so nothing was renamed.

### 2026-10-04T18:49:29.000Z - DOCS AND CONFIG. CLAUDE.md now states the wire format (JSON dotted paths; compact format declined, plans/compact-delta-protocol.md; 093 and 094 as the size levers) and the slot-era file layout; the four backends and the slot model were already described (081, 082). ARCHITECTURE.md's repo section and testing section, README's storage, layout, toolchain and testing lines, and AGENTS.md's sessionId note were corrected. .env.example gains the two test-only variables, and TestEnvExample_DocumentsExactlyWhatIsRead now holds it to exactly what is read (every setting, plus compose-only and test-only variables). CI runs a PostgreSQL 17 service alongside MongoDB with INDRI_TEST_POSTGRES_URI set; the no-database step allows /mongodb and /postgres subtests and two client tests (KNOWN_SKIPS, reasons inline) — the old KNOWN_SKIPS entry named a test that no longer exists. Its filter was run against a real no-database result: nothing unexpected, and removing one exception is caught.

### 2026-10-04T18:49:29.000Z - VERIFICATION. Full comparison in .integration/after-087.txt: go build/vet PASS; go test -race -count=1 ./... with MongoDB and PostgreSQL: 43 packages ok against the baseline's 27, every baseline package still ok, 0 FAIL, 0 races, 0 skips; client clean install under pnpm 11.28.0 with the lockfile unchanged, typecheck PASS, 417/417 tests against 413. No regression.

