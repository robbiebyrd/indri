---
id: 062-e7ab
title: Transport parity and observability for script actions
status: complete
priority: P2
type: feature
created: "2026-09-12T01:28:39.042Z"
updated: "2026-10-08T23:28:51.000Z"
dependencies: ["061"]
plan: plans/lua-game-scripting.md
plan_step: Step 17
depends_on: ["stories/061-3ade-pending-P2-lua-hooks-on-built-in-actions.md"]
started_at: "2026-09-14T01:22:42.095Z"
---

# Transport parity and observability for script actions

## Problem Statement

gqlgen generates a fixed resolver interface at build time, so per-script mutations cannot be added at boot. The repo also has no metrics library, no metrics endpoint and no instrumentation, so a Prometheus-style counter cannot simply be dropped in.

## Acceptance Criteria

- [x] One static generic GraphQL mutation accepting an action name and payload, validated against the engine manifest
- [x] REST exposes script actions with the wildcard payload bounded by maximum key count, depth and bytes, since every existing route is a strict allowlist
- [x] TestRestRoutesMatchRegisteredActions covers script-declared actions
- [x] An equivalent GraphQL parity assertion is added, because nothing catches a missing mutation today
- [x] Kill-reason counters distinguish deadline, string cap, depth cap and script error
- [x] A timeout is classified by checking the context error after PCall, never by ApiError type or string match
- [x] Observability uses expvar or stdlib log, and adding a metrics dependency plus an endpoint is recorded as a separate explicit decision
- [x] VERIFY: go test -race ./...

## Files

- internal/transport/graphql/schema.graphqls
- internal/transport/rest/routes.go
- internal/services/lua/metrics.go

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

### 2026-10-08T23:28:51.000Z - IMPLEMENTED (parity). REST: rest.Transport.ScriptActions (set from Engine.Actions in injector/services.go, before Register) mounts POST /api/<action> per declared action; the body is the payload, whole, bounded by actions.CheckScriptPayload. GraphQL: one static mutation scriptAction(name: String!, payload: JSON): JSON (schema regenerated with gqlgen v0.17.95); the resolver dispatches only a name the engine declared, so built-ins (kick, login) and phases (received) are refused, and bounds the payload the same way. The bound: actions.MaxPayloadDepth 32 and MaxPayloadValues 10000, the Lua conversion budget's own defaultMaxDepth/defaultMaxNodes, counted the same way (payload itself = one value, one level); byte size stays bounded by transport.MaxBodyBytes. TestPayloadLimitsMatchTheConversionBudget ties the constants, TestPayloadLimitsCountLikeTheConversion runs payloads at and past each limit through the real conversion. TestRestRoutesMatchRegisteredActions and TestGraphQLMutationsMatchRegisteredActions now register a script engine (move, pass); GraphQL's is checked by calling scriptAction on a served transport and seeing the script's own "ran:<action>" frame. TestGetServices_ScriptActionsReachRESTAndGraphQL boots the real services over memory stores and the tictactoe config and POSTs /api/move and scriptAction(move): removing either wiring line fails it.

### 2026-10-08T23:28:51.000Z - IMPLEMENTED (observability). Kill reasons deadline, string_cap, depth_cap, node_cap (added: the node cap is its own cap) and script_error. A deadline is read off ctx.Err() after the call, never off the error. A cap is noted on the invocation where it is raised (string.rep/format wrappers; indri.mutate/reply/send via a capError type the marshal budget now returns, messages unchanged), because raising into Lua loses the Go error; pcall/xpcall are wrapped so a caught error clears the note. Counted per engine (Engine.KillCounts) and in the process expvar indri_lua_kills; each kill logged as "lua kill <what> reason=<r> after=<d>". DECISION RECORDED, NOT TAKEN: the expvar is not served over HTTP (the server mounts its own mux, not http.DefaultServeMux), and no metrics library or endpoint was added; exposing one is Boss's separate call. The plan's per-action duration histogram was not in the acceptance criteria and was not built; a kill's duration is logged.

### 2026-10-08T23:28:51.000Z - VERIFY. All new tests failed first. Mutations killed: deadline not read off ctx, cap note ignored, rep cap not noted, reply cap not noted, kill not recorded, pcall wrapper removed, REST bound removed, services wiring removed. One mutant (first cap wins vs last) proved equivalent once caught caps are cleared, and the guard was removed. go test -race ./... green with PostgreSQL and MongoDB; the CI no-database run reports no unexpected skips and no failures.

