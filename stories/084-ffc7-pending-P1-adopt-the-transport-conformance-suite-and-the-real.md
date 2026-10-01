---
id: 084-ffc7
title: Adopt the transport conformance suite and the real-store handler harness
status: pending
priority: P1
type: chore
created: "2026-10-01T13:52:35.315Z"
updated: "2026-10-01T13:52:39.923Z"
dependencies: ["078-ceca"]
plan: plans/main-divergence-integration.md
plan_step: Step 7
depends_on: ["stories/078-ceca-pending-P1-verified-baseline-on-an-integration-branch-off-loc.md"]
---

# Adopt the transport conformance suite and the real-store handler harness

## Problem Statement

Local has five transports each with its own hand-rolled near-duplicate test file. origin/main built one shared conformance suite run against every transport, plus a harness that tests handlers against real in-memory stores instead of fakes. Both are test-infrastructure wins worth taking regardless of any other decision.

## Acceptance Criteria

- [ ] internal/transport/transporttest exists and its suite runs against ws, graphql, sse, rest and webrtc
- [ ] The suite covers ordered one-at-a-time delivery, concurrent-write safety, broadcast filtering by sessionId, and exactly-once disconnect after a kick
- [ ] internal/handlers/handlertest exists and at least one handler test uses real in-memory stores rather than hand-rolled fakes
- [ ] TestRestRoutesMatchRegisteredActions still passes and still fails when a registered action has no route
- [ ] TestGraphQLMutationsMatchRegisteredActions still passes and still fails when a registered action has no mutation
- [ ] VERIFY: go test ./internal/transport/...

## Files

- internal/transport/transporttest/
- internal/handlers/handlertest/

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

