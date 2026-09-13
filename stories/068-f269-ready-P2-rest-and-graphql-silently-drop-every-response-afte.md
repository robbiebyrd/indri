---
id: 068-f269
title: REST and GraphQL silently drop every response after the first
status: ready
priority: P2
type: fix
created: "2026-09-13T18:49:24.478Z"
updated: "2026-09-13T23:03:38.669Z"
dependencies: []
---

# REST and GraphQL silently drop every response after the first

## Problem Statement

router.Dispatch merges results from every handler matching an action (internal/handlers/router/act.go:46 appends Responses), and registration is additive — a game handler registered alongside a built-in, or a received/processed hook, produces more than one response. WebSocket writes them all (internal/services/boot/handlers.go:157), but REST (internal/transport/rest/rest.go:164) and GraphQL (internal/transport/graphql/resolvers/resolver.go:79) both take Responses[0] and discard the rest with no error and no log. Today every built-in returns exactly one response so nothing hits it, but story 061 (Lua hooks on built-in actions) makes multi-handler dispatch routine, and story 054 caps indri.reply at one call specifically to dodge this — which protects scripts without fixing the underlying divergence. Aggregating into an array would change the REST and GraphQL wire contract, so this needs a deliberate decision rather than a quiet fix inside another story.

## Acceptance Criteria

- [ ] A decision is recorded on whether REST and GraphQL aggregate, return only the first with an explicit contract, or error when a dispatch produces more than one response
- [ ] Whatever is chosen, the behaviour is identical across REST, GraphQL and WebSocket, or the divergence is documented in docs/PROTOCOL.md as intentional
- [ ] A test asserts the chosen behaviour on all three transports for a two-handler action
- [ ] If the wire contract changes, client/services is updated to match and docs/PROTOCOL.md records the new shape
- [ ] VERIFY: go test -race ./internal/transport/... ./internal/handlers/router/

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

