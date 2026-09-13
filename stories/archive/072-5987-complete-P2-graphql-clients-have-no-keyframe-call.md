---
id: 072-5987
title: GraphQL clients have no keyframe call
status: complete
priority: P2
type: feature
created: "2026-09-13T23:19:58.243Z"
updated: "2026-09-13T23:27:10.582Z"
dependencies: []
completed_at: "2026-09-13T23:27:10.582Z"
---

# GraphQL clients have no keyframe call

## Problem Statement

REST exposes /api/refresh so a client that misses game state can ask for it again. GraphQL has no refresh mutation at all, which boot/handlers_test.go already records in knownMissingFromGraphQL. This became concrete with 068-f269: reconnect returns two responses and request-response transports deliver only the first, so a GraphQL client resuming into an active game loses its keyframe and then has no way to ask for another. Making the drop loud was the mitigation; this is the repair.

## Acceptance Criteria

- [x] A refresh mutation exists in schema.graphqls with a resolver dispatching the existing refresh action
- [x] A GraphQL client that has lost its keyframe can recover the full sanitised game through it
- [x] refresh is removed from knownMissingFromGraphQL and the parity test passes without the exemption
- [x] The mutation returns the same document REST /api/refresh returns for the same session
- [x] VERIFY: go test -race ./internal/transport/graphql/ ./internal/services/boot/

## Files

- internal/transport/graphql/schema.graphqls
- internal/transport/graphql/resolvers/schema.resolvers.go

## Proof

- [x] [completeness] Completeness (4 of 4 criteria; mutation, resolver, exemption removal and the REST-equivalence test)
- [x] [feature-availability] Feature availability (A GraphQL client that loses reconnect's keyframe can now recover one, which it could not before; the exemption is removed rather than relaxed so the parity test enforces it)
- [x] [robustness] Robustness (Regenerated rather than hand-edited, via an alternate modfile so the codegen-only dependencies never entered go.mod; both files verified unmodified afterwards)
- [x] [resilience] Resilience (RED proved by pointing the resolver at a wrong action name: both mutation tests fail and the parity test fails on both directions of drift)
- [x] [security] Security (The mutation takes no arguments and a dedicated test reads the compiled schema to enforce that, so the caller is resolved from the bearer token and can never name a session or user)
- [x] [defense-in-depth] Defense in depth (The REST-equivalence test asserts both documents equal the action's own bytes, not merely each other, because equality alone would be satisfied by both transports mangling it identically)
- [x] [input-validation] Input validation (Token handling is table-driven over valid, unknown and absent, asserting the session handed to the action is the token's or nil)
- [~] [thread-safety] Thread safety (Stateless resolver dispatching through the existing router)
- [~] [configurability] Configurability (No configuration; the mutation mirrors the existing REST route)

## QA

Verified independently: graphql and boot packages green under -race; go.mod and go.sum confirmed unmodified after the alternate-modfile gqlgen run, and the temp modfiles are gone; knownMissingFromGraphQL is now an empty map with the parity test still failing on drift in both directions. 4/4.

## Work Log

### 2026-09-13T23:26:21.940Z - Added the refresh mutation to schema.graphqls (refresh: JSON!, no arguments) and implemented the generated Refresh resolver as r.dispatch(ctx, "refresh", {}), so the caller is resolved from the bearer token only. Regenerated generated.go and schema.resolvers.go with gqlgen v0.17.95 via an alternate modfile (go run -modfile=gqlgentool.mod), because the codegen-only deps (x/tools, goccy/go-yaml, urfave/cli) are absent from go.sum; go.mod/go.sum are unchanged and the temp modfile was deleted. gqlgen's trailing 'go mod tidy' failed on the sandboxed build cache after codegen had already written both files. Emptied knownMissingFromGraphQL in internal/services/boot/handlers_test.go; the parity test now enforces refresh. New tests in internal/transport/graphql/graphql_test.go: RefreshReturnsTheSameKeyframeAsTheRestRoute drives the real graphql and rest transports over one httptest server each against the same registered refresh action and asserts byte-identical documents; RefreshResolvesTheCallerFromTheBearerTokenAlone is table-driven over valid/unknown/absent tokens; SchemaRefreshTakesNoArguments guards against a gameId/session argument. RED proven by pointing the resolver at a wrong action name. No database, no skips. go build, go vet and go test -race are green everywhere except internal/services/stage, which does not compile due to another agent's in-flight work (stage_test.go:68 undefined: gameService).


### 2026-09-13T23:27:07.400Z - Proof completeness set PROVEN: 4 of 4 criteria; mutation, resolver, exemption removal and the REST-equivalence test

### 2026-09-13T23:27:07.475Z - Proof feature-availability set PROVEN: A GraphQL client that loses reconnect's keyframe can now recover one, which it could not before; the exemption is removed rather than relaxed so the parity test enforces it

### 2026-09-13T23:27:07.552Z - Proof robustness set PROVEN: Regenerated rather than hand-edited, via an alternate modfile so the codegen-only dependencies never entered go.mod; both files verified unmodified afterwards

### 2026-09-13T23:27:07.626Z - Proof resilience set PROVEN: RED proved by pointing the resolver at a wrong action name: both mutation tests fail and the parity test fails on both directions of drift

### 2026-09-13T23:27:07.708Z - Proof security set PROVEN: The mutation takes no arguments and a dedicated test reads the compiled schema to enforce that, so the caller is resolved from the bearer token and can never name a session or user

### 2026-09-13T23:27:07.790Z - Proof defense-in-depth set PROVEN: The REST-equivalence test asserts both documents equal the action's own bytes, not merely each other, because equality alone would be satisfied by both transports mangling it identically

### 2026-09-13T23:27:07.876Z - Proof input-validation set PROVEN: Token handling is table-driven over valid, unknown and absent, asserting the session handed to the action is the token's or nil

### 2026-09-13T23:27:07.960Z - Proof thread-safety set NOT_APPLICABLE: Stateless resolver dispatching through the existing router

### 2026-09-13T23:27:08.043Z - Proof configurability set NOT_APPLICABLE: No configuration; the mutation mirrors the existing REST route
