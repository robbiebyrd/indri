---
id: 069-1d64
title: Carry mutation.RunResult and the lock tests onto the trunk
status: wontfix
priority: P1
type: refactor
created: "2026-10-01T13:29:44.856Z"
updated: "2026-10-01T13:51:32.944Z"
dependencies: ["068-223a"]
plan: plans/main-divergence-integration.md
plan_step: Step 2
depends_on: ["stories/068-223a-pending-P1-establish-a-verified-trunk-baseline-on-the-integra.md"]
completed_at: "2026-10-01T13:51:32.943Z"
---

# Carry mutation.RunResult and the lock tests onto the trunk

## Problem Statement

The Lua engine's GameMutator port needs to know whether a mutation committed, which mutation.RunResult provides. Remote never touched internal/services/mutation or internal/services/lock (git diff --numstat 26a633a origin/main is empty for both), so this is a clean additive carry rather than a merge.

## Acceptance Criteria

- [ ] mutation.RunResult present on the integration branch, returning (committed bool, err error)
- [ ] TestRunResult_DistinguishesAbortFromCommit passes
- [ ] TestRunResult_ReportsNoCommitOnConflict passes
- [ ] TestRunResult_ReportsNoCommitOnApplyError passes
- [ ] The 271 lines of internal/services/lock tests are carried and pass
- [ ] VERIFY: go test ./internal/services/mutation/ ./internal/services/lock/

## Files

- internal/services/mutation/mutation.go
- internal/services/lock/inprocess.go
- internal/services/lock/redis.go

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

### 2026-10-01T13:51:32.865Z - Superseded by the trunk reversal: origin/main lacks the connection-independent dispatch refactor, so local main became the trunk and the port direction inverted. Plan plans/main-divergence-integration.md revised; this story's criteria were written for the opposite direction. Replaced by the revised story set.

