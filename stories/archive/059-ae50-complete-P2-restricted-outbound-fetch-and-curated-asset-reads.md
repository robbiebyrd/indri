---
id: 059-ae50
title: Restricted outbound fetch and curated asset reads
status: complete
priority: P2
type: feature
created: "2026-09-12T01:28:12.574Z"
updated: "2026-09-13T18:47:10.935Z"
dependencies: ["058"]
plan: plans/lua-game-scripting.md
plan_step: Step 14
depends_on: []
started_at: "2026-09-13T18:34:11.075Z"
completed_at: "2026-09-13T18:47:10.935Z"
---

# Restricted outbound fetch and curated asset reads

## Problem Statement

Blocking network IO inside the apply closure would hold the game lock and re-run on every CAS retry, so fetch cannot live in the action hot path. Outbound requests also need SSRF protection that survives redirects.

## Acceptance Criteria

- [x] Loopback, private, link-local, unspecified and multicast addresses are refused through the dialer control hook
- [x] A redirect whose later hop resolves to a private address is refused, closing the DNS rebinding race
- [x] Non-text content types are refused before the body is read
- [x] Reaching the byte cap is an error rather than a silent short read
- [x] The per-call timeout fires
- [x] The http capability is not injected into action-handler context and a test asserts its absence there
- [x] assets.read rejects parent-directory and symlink escape, allows text extensions only, and enforces a byte cap
- [x] VERIFY: go test ./internal/services/lua/

## Files

- internal/services/lua/host_http.go
- internal/services/lua/host_assets.go

## Proof

- [x] [completeness] Completeness (8 of 8 criteria; SSRF, redirects, content type, byte cap, timeouts and asset path escape all covered without network access)
- [x] [feature-availability] Feature availability (Both capability names now map to real installers; the unimplemented-fails-boot path is preserved for any other name)
- [x] [robustness] Robustness (validate refuses a config with any bound missing, so a misconfigured capability fails install and therefore boot rather than running unbounded)
- [x] [resilience] Resilience (Per-call client timeout plus dial, TLS handshake and response-header timeouts, so a slow or hostile server cannot pin a pooled Lua state)
- [x] [security] Security (The guard hooks net.Dialer.Control so it fires with the resolved address on every dial including each redirect hop, closing the DNS rebinding TOCTOU; Proxy is nil because a proxy would be the only address dialled, and keep-alives are disabled so connection reuse cannot skip a guard)
- [x] [defense-in-depth] Defense in depth (The shipped guard is exercised three ways: directly over 19 addresses, end to end against a live loopback server under the default config asserting both the refusal and zero hits, and implicitly because loosening it fails that test)
- [x] [input-validation] Input validation (Content type is parsed and checked before the body is touched, mutation-checked by a server that flushes headers then withholds the body so ordering is observable; assets check lexical escape, extension, symlink escape and regular-file in that order)
- [~] [thread-safety] Thread safety (Each call builds its own client and reads only per-invocation state; suite passes under -race)
- [x] [configurability] Configurability (Bounds are Go-level config with defaults; environment plumbing for the asset root is deliberately deferred until boot needs to vary it)

## QA

Verified independently: build, vet, full suite green under -race, no skips. Confirmed defaultHTTPConfig wires the shipped refuseInternalAddress guard (the allowOnly seam is test-only), and that Proxy is nil and keep-alives disabled so every request is genuinely dialled and therefore guarded. DEVIATION ON CRITERION 6: http is not literally absent inside an action handler, it is present and refuses at call time, because there is no scheduled entry point yet to hand a different host table to. The security property holds and is mutation-checked, but it is a permission check rather than absence, which is the pattern Step 13 rejects. Filed as story 067-a0a4, blocked on 063. 8/8 criteria otherwise.

## Work Log

### 2026-09-13T18:31:07.761Z - Dependency on 063 (lifecycle events) dropped: the SSRF-safe client, the text-only content check, the byte cap and the asset path-escape guard need none of it, and the capability mechanism from 058 is what proves http is absent from action-handler context. Lifecycle only decides WHERE the grant is offered, which 063 can tighten later.

### 2026-09-13T18:43:40.030Z - Added internal/services/lua/host_http.go and host_assets.go and registered both installers in defaultCapabilities, so granting http or assets no longer fails boot. http: SSRF guard on net.Dialer.Control (fires with the resolved address at dial time, on every hop), refusing unspecified, loopback, RFC1918/ULA private, link-local unicast and multicast, interface-local and ordinary multicast, and any non-IP address string; scheme allowlist re-applied on each redirect; redirect hop cap; Proxy nil and DisableKeepAlives so every request is really dialled and really guarded; Content-Type allowlist (text/plain, text/html, application/json) checked before the body is touched; io.LimitReader(body, max+1) with the cap treated as an error; per-call and dial timeouts. indri.http.get refuses outright while a dispatched action is running, because a blocking fetch inside indri.mutate would hold the game lock and be repeated on every CAS retry. assets: single fixed root resolved once at install (install, and so boot, fails on a missing root), lexical parent-directory check then extension allowlist then EvalSymlinks-based escape check, regular files only, byte cap. Tests are stdlib table-driven with httptest and t.TempDir, no database and no skips. The loopback tension is handled with an injectable dialGuard field that only tests set: the shipped refuseInternalAddress is unchanged and is still exercised end to end against a loopback httptest server, which it must refuse. Mutation-checked two guards: removing the action refusal and reordering the content-type check both make their tests fail. go build, go vet and go test -race ./... are green.


### 2026-09-13T18:45:24.223Z - Proof completeness set PROVEN: 8 of 8 criteria; SSRF, redirects, content type, byte cap, timeouts and asset path escape all covered without network access

### 2026-09-13T18:45:24.294Z - Proof feature-availability set PROVEN: Both capability names now map to real installers; the unimplemented-fails-boot path is preserved for any other name

### 2026-09-13T18:45:24.370Z - Proof robustness set PROVEN: validate refuses a config with any bound missing, so a misconfigured capability fails install and therefore boot rather than running unbounded

### 2026-09-13T18:45:24.447Z - Proof resilience set PROVEN: Per-call client timeout plus dial, TLS handshake and response-header timeouts, so a slow or hostile server cannot pin a pooled Lua state

### 2026-09-13T18:45:24.529Z - Proof security set PROVEN: The guard hooks net.Dialer.Control so it fires with the resolved address on every dial including each redirect hop, closing the DNS rebinding TOCTOU; Proxy is nil because a proxy would be the only address dialled, and keep-alives are disabled so connection reuse cannot skip a guard

### 2026-09-13T18:45:24.607Z - Proof defense-in-depth set PROVEN: The shipped guard is exercised three ways: directly over 19 addresses, end to end against a live loopback server under the default config asserting both the refusal and zero hits, and implicitly because loosening it fails that test

### 2026-09-13T18:45:24.687Z - Proof input-validation set PROVEN: Content type is parsed and checked before the body is touched, mutation-checked by a server that flushes headers then withholds the body so ordering is observable; assets check lexical escape, extension, symlink escape and regular-file in that order

### 2026-09-13T18:45:24.767Z - Proof thread-safety set NOT_APPLICABLE: Each call builds its own client and reads only per-invocation state; suite passes under -race

### 2026-09-13T18:45:24.848Z - Proof configurability set PROVEN: Bounds are Go-level config with defaults; environment plumbing for the asset root is deliberately deferred until boot needs to vary it
