---
id: 059-ae50
title: Restricted outbound fetch and curated asset reads
status: pending
priority: P2
type: feature
created: "2026-09-12T01:28:12.574Z"
updated: "2026-09-12T01:29:08.250Z"
dependencies: ["058", "063"]
plan: plans/lua-game-scripting.md
plan_step: Step 14
depends_on: ["stories/058-6453-pending-P2-per-script-capability-injection.md", "stories/063-e85b-pending-P2-game-lifecycle-events.md"]
---

# Restricted outbound fetch and curated asset reads

## Problem Statement

Blocking network IO inside the apply closure would hold the game lock and re-run on every CAS retry, so fetch cannot live in the action hot path. Outbound requests also need SSRF protection that survives redirects.

## Acceptance Criteria

- [ ] Loopback, private, link-local, unspecified and multicast addresses are refused through the dialer control hook
- [ ] A redirect whose later hop resolves to a private address is refused, closing the DNS rebinding race
- [ ] Non-text content types are refused before the body is read
- [ ] Reaching the byte cap is an error rather than a silent short read
- [ ] The per-call timeout fires
- [ ] The http capability is not injected into action-handler context and a test asserts its absence there
- [ ] assets.read rejects parent-directory and symlink escape, allows text extensions only, and enforces a byte cap
- [ ] VERIFY: go test ./internal/services/lua/

## Files

- internal/services/lua/host_http.go
- internal/services/lua/host_assets.go

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

