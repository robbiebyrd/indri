---
id: 071-4bcb
title: Cap the GraphQL request body like REST and WebSocket already do
status: complete
priority: P2
type: fix
created: "2026-09-13T18:49:29.773Z"
updated: "2026-09-13T18:56:14.651Z"
dependencies: []
completed_at: "2026-09-13T18:56:14.651Z"
---

# Cap the GraphQL request body like REST and WebSocket already do

## Problem Statement

internal/transport/rest/rest.go wraps bodies in http.MaxBytesReader at 64 KiB and internal/transport/ws/ws.go sets MaxMessageSize, but the GraphQL handler is mounted as mux.Handle(path, authMiddleware(t.srv)) with no cap, and internal/entrypoints/http/server.go sets only ReadTimeout. Layout ops carrying a script source or an opaque config are exactly the large payload fields this leaves unguarded.

## Acceptance Criteria

- [x] The GraphQL handler is wrapped in a body size limit consistent with the REST limit
- [x] A request over the limit is rejected before the resolver runs
- [x] A normal request is unaffected
- [x] The limit is expressed once rather than duplicated per transport if that is practical
- [x] VERIFY: go test -race ./internal/transport/graphql/

## Files

- internal/transport/graphql/graphql.go

## Proof

- [x] [completeness] Completeness (5 of 5 criteria; over-limit, at-limit, inside-limit, chunked-over and subscription-upgrade all covered)
- [x] [feature-availability] Feature availability (The cap is outermost on the GraphQL route, so an oversized operation is refused before it is read, authenticated, parsed or resolved)
- [x] [robustness] Robustness (Bare http.MaxBytesHandler would have answered 200 OK, because gqlgen catches the read error and writes its own error document with no explicit status; the explicit Content-Length check returns a truthful 413)
- [x] [resilience] Resilience (The subscription upgrade is proved alive by completing a connection_init to connection_ack round trip, not merely by asserting a 101)
- [x] [security] Security (Closes the one transport that had no inbound size discipline while REST capped at 64 KiB and WebSocket set MaxMessageSize)
- [x] [defense-in-depth] Defense in depth (Chunked bodies of unknown length still hit MaxBytesReader, so a request that omits Content-Length is not a way around the cap)
- [x] [input-validation] Input validation (Both tests were shown non-vacuous by reverting: without the wrapper the over-limit cases return 200 with the resolver marker present)
- [~] [thread-safety] Thread safety (Stateless middleware over per-request values)
- [x] [configurability] Configurability (MaxBodyBytes now lives once in internal/transport beside the other cross-adapter pieces rather than duplicated per adapter)

## QA

Verified independently: transport, graphql and rest packages green under -race. Confirmed the declared Content-Length check returns 413 without reading the body, and that the ResponseWriter is passed through unwrapped so the subscription upgrade stays hijackable. 5/5.

## Work Log

### 2026-09-13T18:54:33.936Z - Capped the GraphQL request body. Moved the 64 KiB limit out of rest.go into internal/transport as MaxBodyBytes, alongside a LimitBody handler wrapper that refuses a declared Content-Length over the cap with 413 and falls back to http.MaxBytesReader for chunked bodies; rest.decodeBody now reads the shared constant. graphql.Register mounts transport.LimitBody outside authMiddleware, so an oversized operation is refused before gqlgen parses or authenticates it. LimitBody passes the ResponseWriter through unwrapped, so the gameUpdates subscription upgrade still hijacks. Tests: table-driven TestLimitBody in internal/transport (declared, chunked, at/over the limit), TestRegister_BodyLimit in the graphql package driving a real httptest server (ordinary, just inside, over, and chunked-over - proven to fail without the wrapper, and the { ping } resolver's pong is the marker that nothing executed), and TestRegister_BodyLimitLeavesTheSubscriptionUpgradeAlone which completes a graphql-transport-ws connection_init/ack (proven to fail if the ResponseWriter is wrapped). go build, go vet and go test -race ./... all green.


### 2026-09-13T18:56:13.979Z - Proof completeness set PROVEN: 5 of 5 criteria; over-limit, at-limit, inside-limit, chunked-over and subscription-upgrade all covered

### 2026-09-13T18:56:14.048Z - Proof feature-availability set PROVEN: The cap is outermost on the GraphQL route, so an oversized operation is refused before it is read, authenticated, parsed or resolved

### 2026-09-13T18:56:14.121Z - Proof robustness set PROVEN: Bare http.MaxBytesHandler would have answered 200 OK, because gqlgen catches the read error and writes its own error document with no explicit status; the explicit Content-Length check returns a truthful 413

### 2026-09-13T18:56:14.192Z - Proof resilience set PROVEN: The subscription upgrade is proved alive by completing a connection_init to connection_ack round trip, not merely by asserting a 101

### 2026-09-13T18:56:14.264Z - Proof security set PROVEN: Closes the one transport that had no inbound size discipline while REST capped at 64 KiB and WebSocket set MaxMessageSize

### 2026-09-13T18:56:14.335Z - Proof defense-in-depth set PROVEN: Chunked bodies of unknown length still hit MaxBytesReader, so a request that omits Content-Length is not a way around the cap

### 2026-09-13T18:56:14.407Z - Proof input-validation set PROVEN: Both tests were shown non-vacuous by reverting: without the wrapper the over-limit cases return 200 with the resolver marker present

### 2026-09-13T18:56:14.482Z - Proof thread-safety set NOT_APPLICABLE: Stateless middleware over per-request values

### 2026-09-13T18:56:14.555Z - Proof configurability set PROVEN: MaxBodyBytes now lives once in internal/transport beside the other cross-adapter pieces rather than duplicated per adapter
