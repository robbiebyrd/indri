---
id: 074-dd4e
title: Decide the transport stack between the two implementations
status: wontfix
priority: P1
type: refactor
created: "2026-10-01T13:29:44.860Z"
updated: "2026-10-01T13:51:33.865Z"
dependencies: ["071-3816"]
plan: plans/main-divergence-integration.md
plan_step: Step 7
depends_on: ["stories/071-3816-pending-P1-port-the-lua-engine-and-scheduler-onto-the-trunk-s.md"]
completed_at: "2026-10-01T13:51:33.864Z"
---

# Decide the transport stack between the two implementations

## Problem Statement

Both sides built the transport plumbing independently: transport.Multi vs Composite, Keys/Registry/BufferedConn vs hub.go, gqlgen GraphQL at 4,517 lines vs graphqlws at 403, and a separate rest package vs SSE+POST folded into sse. These cannot be blended; one of each pair must win, and the choice decides whether the GraphQL parity tests stay meaningful.

## Acceptance Criteria

- [ ] A diff summary comparing each pair is produced and a recommendation recorded before any file is overwritten
- [ ] One implementation of each pair is chosen; no blended hybrid is created
- [ ] The decision and what was dropped is recorded in the story worklog
- [ ] TestRestRoutesMatchRegisteredActions passes and still fails when a registered action has no route
- [ ] TestGraphQLMutationsMatchRegisteredActions passes and still fails when a registered action has no mutation
- [ ] VERIFY: go test ./internal/transport/... ./internal/services/boot/

## Files

- internal/transport/
- internal/transport/graphql/
- internal/transport/graphqlws/
- internal/transport/rest/

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

### 2026-10-01T13:34:40.807Z - Decision analysis dispatched (read-only, pre-branch): comparing Multi/Composite, shared plumbing vs hub.go, gqlgen vs graphqlws, rest vs SSE+POST. Also verifying both parity tests exist on each ref and whether origin/main shares the script-action blind spot.

### 2026-10-01T13:40:42.657Z - Transport analysis returned. Key correction: the agent claimed origin/main has no Lua engine at all -- FALSE. Verified origin/main has internal/handlers/luahandler (8 files), gopher-lua v1.1.2 in go.mod, wired at boot.go:54. Its downstream reasoning about story 062 is therefore unsound. Verified findings that DO hold: origin/main has neither parity test (zero hits for either name); origin's graphqlws is framing-only with no schema (its own header comment); origin's origin.go is open-by-default where local fails closed; origin has no MessagePack counterpart in local.

### 2026-10-01T13:51:33.799Z - Superseded by the trunk reversal: origin/main lacks the connection-independent dispatch refactor, so local main became the trunk and the port direction inverted. Plan plans/main-divergence-integration.md revised; this story's criteria were written for the opposite direction. Replaced by the revised story set.

