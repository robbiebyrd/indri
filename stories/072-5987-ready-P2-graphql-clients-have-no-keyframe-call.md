---
id: 072-5987
title: GraphQL clients have no keyframe call
status: ready
priority: P2
type: feature
created: "2026-09-13T23:19:58.243Z"
updated: "2026-09-13T23:20:01.705Z"
dependencies: []
---

# GraphQL clients have no keyframe call

## Problem Statement

REST exposes /api/refresh so a client that misses game state can ask for it again. GraphQL has no refresh mutation at all, which boot/handlers_test.go already records in knownMissingFromGraphQL. This became concrete with 068-f269: reconnect returns two responses and request-response transports deliver only the first, so a GraphQL client resuming into an active game loses its keyframe and then has no way to ask for another. Making the drop loud was the mitigation; this is the repair.

## Acceptance Criteria

- [ ] A refresh mutation exists in schema.graphqls with a resolver dispatching the existing refresh action
- [ ] A GraphQL client that has lost its keyframe can recover the full sanitised game through it
- [ ] refresh is removed from knownMissingFromGraphQL and the parity test passes without the exemption
- [ ] The mutation returns the same document REST /api/refresh returns for the same session
- [ ] VERIFY: go test -race ./internal/transport/graphql/ ./internal/services/boot/

## Files

- internal/transport/graphql/schema.graphqls
- internal/transport/graphql/resolvers/schema.resolvers.go

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

