---
id: 086-59e8
title: Settle the origin policy and harvest the remaining trunk fixes
status: pending
priority: P1
type: task
created: "2026-10-01T13:52:35.318Z"
updated: "2026-10-01T13:52:40.083Z"
dependencies: ["084-ffc7"]
plan: plans/main-divergence-integration.md
plan_step: Step 9
depends_on: ["stories/084-ffc7-pending-P1-adopt-the-transport-conformance-suite-and-the-real.md"]
---

# Settle the origin policy and harvest the remaining trunk fixes

## Problem Statement

origin/main deliberately changed the origin policy to accept every origin when no allowlist is set; local fails closed. The trunk's own documentation admits an open default lets any page reach a localhost or private-network server through a visitor's browser. That posture must not be adopted by accident during a port. Several smaller trunk improvements are worth taking at the same time.

## Acceptance Criteria

- [ ] Local's fail-closed OriginPolicy is retained and origin_test.go proves an unlisted cross-origin request is still refused by default
- [ ] If an open-by-default mode is wanted at all, it is an explicit opt-in with its own documentation, not the default
- [ ] ?debug=1 selects JSON on every transport
- [ ] CLI flags exist for every setting, shared between the server and the tictactoe example (origin 995b144)
- [ ] pnpm, node and go versions are pinned so local matches CI (origin 6650363)
- [ ] The empty game list no longer crashes the Join screen (origin 21113cb)
- [ ] VERIFY: go test ./internal/transport/ && go build ./...

## Files

- internal/transport/origin.go
- internal/transport/origin_test.go
- internal/cli/
- client/app/index.tsx

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

