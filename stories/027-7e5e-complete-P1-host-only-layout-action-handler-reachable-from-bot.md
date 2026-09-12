---
id: 027-7e5e
title: Host-only layout action handler, reachable from both transports
status: complete
priority: P1
type: feature
created: "2026-09-10T00:58:00.964Z"
updated: "2026-09-12T01:28:08.440Z"
dependencies: ["025", "026"]
plan: plans/layout-authoring-editor.md
plan_step: Step 3
depends_on: ["stories/025-6b2f-pending-P2-layout-op-envelope-types-and-decoding.md", "stories/026-0ae1-pending-P2-structural-layout-validation-in-go.md"]
started_at: "2026-09-12T01:12:44.492Z"
completed_at: "2026-09-12T01:28:08.440Z"
---

# Host-only layout action handler, reachable from both transports

## Problem Statement

This action lets a client write game state, so it is the security boundary of the whole authoring feature. Since the transport refactor the caller arrives pre-authenticated as req.Session, which makes payload spoofing structurally impossible rather than a rule each handler must remember. The action must also exist in BOTH transports: nothing in CI catches a WS-registered action missing from schema.graphqls, so it would be silently unreachable for GraphQL clients.

## Acceptance Criteria

- [x] VERIFY: go test -race ./...
- [x] Handler implements actions.MessageHandler: Handle(actions.Request) (actions.Result, error). No melody, no connection service, no GetKeyAsString
- [x] A nil req.Session is rejected; a session with no UserID is rejected; a session whose GameID differs from the target game is rejected; a non-host caller is rejected
- [x] A userId present in req.Payload claiming host is ignored, pinned by a test so it stays a non-path
- [x] A valid addWidget mutates ONLY data.layout and leaves players, teams, stage and privateData byte-identical
- [x] removeWidget on a missing id returns mutation.ErrAbort so no version bump and no delta occur
- [x] Mutate is called with no context and no explicit publish, since it diffs and publishes on commit
- [x] Registered in boot.registerHandlers so TestRegisterHandlers_CoversEveryActionPackage passes
- [x] A layout mutation exists in internal/transport/graphql/schema.graphqls with a one-line r.dispatch resolver, and gqlgen has been regenerated
- [x] Errors are returned rather than written to a connection; the transport decides how to surface them

## Files

- internal/handlers/actions/layout/handler.go
- internal/handlers/actions/layout/handler_test.go
- internal/services/boot/handlers.go
- internal/transport/graphql/schema.graphqls

## Related

- 009

## Proof

- [x] [completeness] Completeness (All 10 criteria checked. go build, go vet, gofmt and go test -race clean except the pre-existing graphql origin-policy failure, untouched.)
- [x] [feature-availability] Feature availability (Reachable from all three transports: registered in boot.registerHandlers （coverage test green）, a layout mutation in schema.graphqls with a regenerated resolver, and a /api route in rest/routes.go （TestRestRoutesMatchRegisteredActions green）.)
- [x] [robustness] Robustness (removeWidget on a missing id returns mutation.ErrAbort so no version bump and no delta occur. Mutate is called with no context and no explicit publish, so the delta is not double-broadcast.)
- [x] [resilience] Resilience (Errors are returned rather than written to a connection, so each transport decides how to surface them. No panics on malformed payloads.)
- [x] [security] Security (Caller comes from req.Session only; a payload userId is never read. Mutation-verified: disabling the host check fails BOTH TestEditLayout_RejectsANonHost and TestEditLayout_IgnoresAUserIdInThePayload. The latter was strengthened after mutation showed it had been passing on an unrelated gate （decodeOp's unknown-key rejection）.)
- [x] [defense-in-depth] Defense in depth (Four independent gates: transport authentication, host authorization, per-op decode with unknown-key rejection, and validateLayout against the RESULTING document. Authorization runs before decode so an unauthorized caller learns nothing about the op vocabulary.)
- [x] [input-validation] Input validation (applyLayoutOp works on a detached copy and assigns back only after validateLayout passes, so a rejected op leaves the game byte-identical - asserted by comparing marshalled players, teams, stage, privateData and an unrelated public-data key.)
- [x] [thread-safety] Thread safety (All mutation goes through GameRepo.Mutate, which holds the distributed lock and the version fence; this handler adds no state of its own.)
- [~] [configurability] Configurability (The op vocabulary and the host-only rule are the protocol; a knob on either would be a security hole.)

## QA

- [ ] go build, go vet, gofmt, go test -race all clean for this work
- [ ] Host authorization mutation-verified; spoofing test strengthened after it was found passing on an unrelated gate
- [ ] Pre-existing internal/transport/graphql origin-policy failure remains, untouched and not mine

## Work Log

### 2026-09-12T01:27:48.159Z - Handler, apply, registration, GraphQL mutation + resolver + regenerated gqlgen, and a REST route (the rest transport landed mid-flight and TestRestRoutesMatchRegisteredActions would otherwise fail). The design that makes this testable: editLayout takes two narrow interfaces (gameLookup, gameMutator) rather than the injector, so the authorization matrix is exercised with fakes and no Mongo. Dependencies resolve in Handle rather than New, because registerHandlers is exercised with a partial injector by the coverage test and a constructor that dereferenced repos would panic there. Authorization order is deliberate: authenticated -> in the game -> host -> only THEN decode the op, so a caller who may not edit learns nothing about which ops exist. applyLayoutOp works on a DETACHED COPY and assigns back only after validateLayout passes, so a rejected op leaves the game byte-identical. This session was interrupted mid-story and I initially believed handler_test.go was missing and wrote my own; the agent's version landed afterwards and is better (reuses the package's existing helpers), so I kept it. FINDING I fixed: TestEditLayout_IgnoresAUserIdInThePayload passed for the WRONG REASON. Disabling the host check entirely did not fail it, because decodeOp separately rejects 'userId' as an unknown key - the test was pinned on an unrelated gate rather than on authorization. Now asserts the error came from the HOST check specifically; mutation re-verified, and disabling the host check now fails BOTH that test and RejectsANonHost. Tree state: go build, go vet, gofmt and go test -race all clean except the pre-existing internal/transport/graphql origin-policy failure, which is the other session's uncommitted work and which I did not touch.


### 2026-09-12T01:28:03.241Z - Proof completeness set PROVEN: All 10 criteria checked. go build, go vet, gofmt and go test -race clean except the pre-existing graphql origin-policy failure, untouched.

### 2026-09-12T01:28:03.316Z - Proof security set PROVEN: Caller comes from req.Session only; a payload userId is never read. Mutation-verified: disabling the host check fails BOTH TestEditLayout_RejectsANonHost and TestEditLayout_IgnoresAUserIdInThePayload. The latter was strengthened after mutation showed it had been passing on an unrelated gate (decodeOp's unknown-key rejection).

### 2026-09-12T01:28:03.393Z - Proof defense-in-depth set PROVEN: Four independent gates: transport authentication, host authorization, per-op decode with unknown-key rejection, and validateLayout against the RESULTING document. Authorization runs before decode so an unauthorized caller learns nothing about the op vocabulary.

### 2026-09-12T01:28:03.487Z - Proof input-validation set PROVEN: applyLayoutOp works on a detached copy and assigns back only after validateLayout passes, so a rejected op leaves the game byte-identical - asserted by comparing marshalled players, teams, stage, privateData and an unrelated public-data key.

### 2026-09-12T01:28:03.573Z - Proof robustness set PROVEN: removeWidget on a missing id returns mutation.ErrAbort so no version bump and no delta occur. Mutate is called with no context and no explicit publish, so the delta is not double-broadcast.

### 2026-09-12T01:28:03.657Z - Proof feature-availability set PROVEN: Reachable from all three transports: registered in boot.registerHandlers (coverage test green), a layout mutation in schema.graphqls with a regenerated resolver, and a /api route in rest/routes.go (TestRestRoutesMatchRegisteredActions green).

### 2026-09-12T01:28:03.737Z - Proof resilience set PROVEN: Errors are returned rather than written to a connection, so each transport decides how to surface them. No panics on malformed payloads.

### 2026-09-12T01:28:03.817Z - Proof thread-safety set PROVEN: All mutation goes through GameRepo.Mutate, which holds the distributed lock and the version fence; this handler adds no state of its own.

### 2026-09-12T01:28:03.898Z - Proof configurability set NOT_APPLICABLE: The op vocabulary and the host-only rule are the protocol; a knob on either would be a security hole.
