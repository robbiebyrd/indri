---
id: 097-a3f7
title: Revisit the open-by-default origin policy
status: complete
priority: P2
type: task
created: "2026-10-10T01:14:21.000Z"
updated: "2026-11-17T21:38:49.000Z"
dependencies: ["086-59e8"]
completed_at: "2026-11-17T21:38:49.000Z"
---

# Revisit the open-by-default origin policy

## Problem Statement

Story 086 adopted main's origin policy by Boss's decision: with `INDRI_ALLOWED_ORIGINS` empty, every browser Origin may open a WebSocket, GraphQL upgrade or CORS request. When a list is set, the policy is exactly those origins plus clients that send no Origin. The security reviewer flagged the open default. Connections carry no cookies or ambient credentials, so a foreign page cannot act as an existing player. But any web page a visitor opens can reach an Indri server on their localhost or private network through the visitor's browser. It can open sockets (each now cost-bounded by the 4002 login deadline), register users and create games. The risk is documented in .env.example and docs/ARCHITECTURE.md, but the default is still open.

## Questions for Boss

1. Keep the open default (status quo, recorded as accepted risk), or change it?
2. If changing: fail closed when the list is empty (refuse every browser Origin, allow no-Origin clients)? That breaks `pnpm run web` until a developer sets the list.
3. Or a middle ground: open by default only when bound to a loopback address, closed otherwise? Or the reverse? Note that loopback is exactly the case the reviewer flagged.
4. Or require an explicit opt-in to open mode, e.g. `INDRI_ALLOWED_ORIGINS="*"`, with an empty value refusing and boot logging the policy in force?
5. Should boot log a warning when running open?
6. Does main need the same change? The policy came from main's 512079a, and diverging here makes the coming rebase (story 099) conflict on it.

## Acceptance Criteria

- [x] Boss's answers recorded in the work log
- [x] If changed: origin_test.go and the GraphQL upgrade tests prove the new default, failing first
- [x] .env.example, docs/ARCHITECTURE.md, docs/PROTOCOL.md and CLAUDE.md (OriginPolicy bullet) describe the policy in force
- [x] VERIFY: go test -race ./...

## Work Log

### 2026-11-17T21:38:49.000Z - BOSS'S ANSWERS (2026-11-17). (1) Change it. (4) Require an explicit opt-in: INDRI_ALLOWED_ORIGINS="*" is open mode, and empty refuses every browser origin. (5) Boot logs the policy in force, and warns when it is open. (6) Main needs nothing separate: the integration branch is now main (story 099).

### 2026-11-17T21:38:49.000Z - IMPLEMENTED (05a5e5b). OriginPolicy: empty = closed (no browser origin, the server's own included; no-Origin clients still pass); a list is unchanged; "*" sets open, and wins over any other entry. New Open() and String(); internal/entrypoints/http logs "origin policy <String()>" at boot, as a WARNING when open. .env.example now lists http://localhost:8081 so the `pnpm run web` quickstart keeps working; README, ARCHITECTURE, PROTOCOL and CLAUDE.md describe the three modes.

### 2026-11-17T21:38:49.000Z - TESTS, RED FIRST. origin_test.go: WithoutAListRefusesEveryBrowserOrigin, StarAcceptsEveryOrigin ("*", " * ", and "*" in a list), String_NamesThePolicyInForce (with Open()) -- failed to build first (no String/Open). graphql_test.go's subscription-upgrade table gained no-list cross-origin, no-list same-origin, no-list native, and "*" cases; before the change the two no-list browser cases connected and the "*" case got 400. All pass now; go test -race ./internal/transport/... ./internal/entrypoints/... green, then the full go test -race ./... with MongoDB and PostgreSQL.
