---
id: 075-c1d4
title: Decide the server layout action between the two implementations
status: wontfix
priority: P1
type: refactor
created: "2026-10-01T13:29:44.861Z"
updated: "2026-10-01T13:51:34.031Z"
dependencies: ["074-dd4e"]
plan: plans/main-divergence-integration.md
plan_step: Step 8
depends_on: ["stories/074-dd4e-pending-P1-decide-the-transport-stack-between-the-two-impleme.md"]
completed_at: "2026-10-01T13:51:34.030Z"
---

# Decide the server layout action between the two implementations

## Problem Statement

Both sides implemented internal/handlers/actions/layout from the same plan with the same filenames: local 3,460 lines across 8 files with a separate apply.go, remote 2,168 across 7. validate.go is 662 lines locally against 229 on the trunk. The Go validator is the security boundary, so choosing on line count rather than on rule coverage would silently drop validation.

## Acceptance Criteria

- [ ] The two validators are compared rule by rule, not by file size, and the comparison is recorded
- [ ] privateData is rejected at any depth including inside arrays, matching the stricter of the two
- [ ] The AABB overlap test, bounds, depth and size caps are all preserved from whichever side had them
- [ ] Remote's slot-aware and multi-backend fixes survive: host is found by slot, layout edits build on the stored layout on every backend, and each game gets its own layout
- [ ] The layout action remains host-only and authorises from the caller's own session, never a client-supplied id
- [ ] VERIFY: go test ./internal/handlers/actions/layout/

## Files

- internal/handlers/actions/layout/validate.go
- internal/handlers/actions/layout/handler.go
- internal/handlers/actions/layout/op.go
- internal/handlers/actions/layout/apply.go

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

### 2026-10-01T13:34:40.881Z - Decision analysis dispatched (read-only, pre-branch): rule-by-rule validator matrix (local 662 vs trunk 229 lines), op vocabulary, host-by-slot authorisation, and the five remote-only fixes that must survive (0f85d3b, 4863a54, e465933, 59bc48c, 40ddd2f).

### 2026-10-01T13:40:42.730Z - Layout analysis returned: local's validator is materially stricter -- trunk lacks unknown-key rejection entirely, does not require type/placement/kind, silently skips a placement-less widget, has no percentage-format check, and rejects a legitimate numeric z on absolute placement. Trunk's addWidget silently overwrites an existing id; four ops upsert rather than edit. Recommends local as base. CAVEAT: e465933's layout-frame machinery exists only because trunk deltas are positional -- that dependency needs weighing against the dispatch-model blocker found on 071.

### 2026-10-01T13:51:33.966Z - Superseded by the trunk reversal: origin/main lacks the connection-independent dispatch refactor, so local main became the trunk and the port direction inverted. Plan plans/main-divergence-integration.md revised; this story's criteria were written for the opposite direction. Replaced by the revised story set.

