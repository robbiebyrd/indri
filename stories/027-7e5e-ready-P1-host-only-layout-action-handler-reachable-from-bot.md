---
id: 027-7e5e
title: Host-only layout action handler, reachable from both transports
status: ready
priority: P1
type: feature
created: "2026-09-10T00:58:00.964Z"
updated: "2026-09-10T00:58:08.818Z"
dependencies: ["025", "026"]
plan: plans/layout-authoring-editor.md
plan_step: Step 3
depends_on: ["stories/025-6b2f-pending-P2-layout-op-envelope-types-and-decoding.md", "stories/026-0ae1-pending-P2-structural-layout-validation-in-go.md"]
---

# Host-only layout action handler, reachable from both transports

## Problem Statement

This action lets a client write game state, so it is the security boundary of the whole authoring feature. Since the transport refactor the caller arrives pre-authenticated as req.Session, which makes payload spoofing structurally impossible rather than a rule each handler must remember. The action must also exist in BOTH transports: nothing in CI catches a WS-registered action missing from schema.graphqls, so it would be silently unreachable for GraphQL clients.

## Acceptance Criteria

- [ ] VERIFY: go test -race ./... 
- [ ] Handler implements actions.MessageHandler: Handle(actions.Request) (actions.Result, error). No melody, no connection service, no GetKeyAsString
- [ ] A nil req.Session is rejected; a session with no UserID is rejected; a session whose GameID differs from the target game is rejected; a non-host caller is rejected
- [ ] A userId present in req.Payload claiming host is ignored, pinned by a test so it stays a non-path
- [ ] A valid addWidget mutates ONLY data.layout and leaves players, teams, stage and privateData byte-identical
- [ ] removeWidget on a missing id returns mutation.ErrAbort so no version bump and no delta occur
- [ ] Mutate is called with no context and no explicit publish, since it diffs and publishes on commit
- [ ] Registered in boot.registerHandlers so TestRegisterHandlers_CoversEveryActionPackage passes
- [ ] A layout mutation exists in internal/transport/graphql/schema.graphqls with a one-line r.dispatch resolver, and gqlgen has been regenerated
- [ ] Errors are returned rather than written to a connection; the transport decides how to surface them

## Files

- internal/handlers/actions/layout/handler.go
- internal/handlers/actions/layout/handler_test.go
- internal/services/boot/handlers.go
- internal/transport/graphql/schema.graphqls

## Related

- 009

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

