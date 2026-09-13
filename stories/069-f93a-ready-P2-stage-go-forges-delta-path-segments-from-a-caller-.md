---
id: 069-f93a
title: stage.go forges delta path segments from a caller-supplied sceneId
status: ready
priority: P2
type: fix
created: "2026-09-13T23:03:08.847Z"
updated: "2026-09-13T23:03:38.773Z"
dependencies: []
---

# stage.go forges delta path segments from a caller-supplied sceneId

## Problem Statement

Story 070 escaped delta path segments in events.joinPath, but internal/services/stage/stage.go builds paths by raw concatenation from a caller-supplied sceneId and path (around lines 63 and 232-236) and hands the result to UpdateField, where the same string is both the Mongo dollar-set path and the published delta path. A dotted sceneId therefore still forges a segment on that route, which is the same defect 070 closed elsewhere. It cannot simply be escaped, because Mongo would then see the backslashes, so it needs key validation at that boundary instead. Note stage.Service is also not currently wired into the injector, so this is reachable only once it is.

## Acceptance Criteria

- [ ] A sceneId or path segment containing a dot, a dollar sign, or a leading dollar sign is rejected before it is concatenated into a Mongo or delta path
- [ ] The rejection names the offending value and returns an error rather than panicking or silently continuing
- [ ] Valid ids are unaffected and the existing stage behaviour is unchanged
- [ ] A regression test covers a dotted sceneId specifically, mirroring the one added for events in story 070
- [ ] VERIFY: go test -race ./internal/services/stage/

## Files

- internal/services/stage/stage.go

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

