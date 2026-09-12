---
id: 062-e7ab
title: Transport parity and observability for script actions
status: pending
priority: P2
type: feature
created: "2026-09-12T01:28:39.042Z"
updated: "2026-09-12T01:29:08.643Z"
dependencies: ["061"]
plan: plans/lua-game-scripting.md
plan_step: Step 17
depends_on: ["stories/061-3ade-pending-P2-lua-hooks-on-built-in-actions.md"]
---

# Transport parity and observability for script actions

## Problem Statement

gqlgen generates a fixed resolver interface at build time, so per-script mutations cannot be added at boot. The repo also has no metrics library, no metrics endpoint and no instrumentation, so a Prometheus-style counter cannot simply be dropped in.

## Acceptance Criteria

- [ ] One static generic GraphQL mutation accepting an action name and payload, validated against the engine manifest
- [ ] REST exposes script actions with the wildcard payload bounded by maximum key count, depth and bytes, since every existing route is a strict allowlist
- [ ] TestRestRoutesMatchRegisteredActions covers script-declared actions
- [ ] An equivalent GraphQL parity assertion is added, because nothing catches a missing mutation today
- [ ] Kill-reason counters distinguish deadline, string cap, depth cap and script error
- [ ] A timeout is classified by checking the context error after PCall, never by ApiError type or string match
- [ ] Observability uses expvar or stdlib log, and adding a metrics dependency plus an endpoint is recorded as a separate explicit decision
- [ ] VERIFY: go test -race ./...

## Files

- internal/transport/graphql/schema.graphqls
- internal/transport/rest/routes.go
- internal/services/lua/metrics.go

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

