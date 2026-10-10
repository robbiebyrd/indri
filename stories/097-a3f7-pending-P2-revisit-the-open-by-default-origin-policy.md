---
id: 097-a3f7
title: Revisit the open-by-default origin policy
status: pending
priority: P2
type: task
created: "2026-10-10T01:14:21.000Z"
updated: "2026-10-10T01:14:21.000Z"
dependencies: ["086-59e8"]
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

- [ ] Boss's answers recorded in the work log
- [ ] If changed: origin_test.go and the GraphQL upgrade tests prove the new default, failing first
- [ ] .env.example, docs/ARCHITECTURE.md, docs/PROTOCOL.md and CLAUDE.md (OriginPolicy bullet) describe the policy in force
- [ ] VERIFY: go test -race ./...

## Work Log
