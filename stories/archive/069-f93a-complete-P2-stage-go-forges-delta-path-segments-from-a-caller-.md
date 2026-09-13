---
id: 069-f93a
title: stage.go forges delta path segments from a caller-supplied sceneId
status: complete
priority: P2
type: fix
created: "2026-09-13T23:03:08.847Z"
updated: "2026-09-13T23:12:18.301Z"
dependencies: []
completed_at: "2026-09-13T23:12:18.301Z"
---

# stage.go forges delta path segments from a caller-supplied sceneId

## Problem Statement

Story 070 escaped delta path segments in events.joinPath, but internal/services/stage/stage.go builds paths by raw concatenation from a caller-supplied sceneId and path (around lines 63 and 232-236) and hands the result to UpdateField, where the same string is both the Mongo dollar-set path and the published delta path. A dotted sceneId therefore still forges a segment on that route, which is the same defect 070 closed elsewhere. It cannot simply be escaped, because Mongo would then see the backslashes, so it needs key validation at that boundary instead. Note stage.Service is also not currently wired into the injector, so this is reachable only once it is.

## Acceptance Criteria

- [x] A sceneId or path segment containing a dot, a dollar sign, or a leading dollar sign is rejected before it is concatenated into a Mongo or delta path
- [x] The rejection names the offending value and returns an error rather than panicking or silently continuing
- [x] Valid ids are unaffected and the existing stage behaviour is unchanged
- [x] A regression test covers a dotted sceneId specifically, mirroring the one added for events in story 070
- [x] VERIFY: go test -race ./internal/services/stage/

## Files

- internal/services/stage/stage.go

## Proof

- [x] [completeness] Completeness (5 of 5 criteria; five entry points covered, three of which the story did not name, plus dollar, leading-dollar, leading-dot, trailing-dot and empty)
- [x] [feature-availability] Feature availability (Validation runs before any store call on every route that composes a path from a caller-supplied key)
- [x] [robustness] Robustness (The recording fake asserts a rejected id never reaches the store, so the guard fails closed rather than rejecting after writing)
- [~] [resilience] Resilience (Pure input validation with no failure path of its own)
- [x] [security] Security (A dotted scene id can no longer forge a segment in the mongo update path or the published delta, which is the same defect 070 closed for events on a route escaping cannot reach)
- [x] [defense-in-depth] Defense in depth (validateAndFetchGame covers SetCurrentScene as well, which needs no validation today but composes a path from the same caller-supplied id)
- [x] [input-validation] Input validation (UpdateScene's optional sub-path is a dotted path by design, so it is split and each segment validated rather than rejecting dots outright and breaking legitimate nesting)
- [~] [thread-safety] Thread safety (Stateless validation over caller-supplied strings)
- [~] [configurability] Configurability (Rejection rule is fixed, not tunable)

## QA

Verified independently: stage package green under -race, zero skips. Confirmed all five entry points validate before any store call, and confirmed the reported typo at stage.go:222 (stage.scene singular vs stage.scenes everywhere else). Agent proved RED by neutering validateSegment and printing the forged paths actually written. Found three concatenation sites beyond the two the story named. 5/5.

## Work Log

### 2026-09-13T23:11:16.986Z - Validated caller-supplied path segments in internal/services/stage before they are concatenated into a Mongo update path. Added ErrInvalidPathSegment plus validateSegment/validatePath: a segment that is empty or contains '.' or '$' is rejected, naming the offending value and wrapping the sentinel error. Wired into AddScene (so AddScenes too), validateAndFetchGame (DeleteScene, SetCurrentScene), LoadSceneFromScript and UpdateScene, in every case before any store call. UpdateScene's optional sub-path is treated as a dotted path and validated per segment, so legitimate nesting still works. Escaping is not usable on this route because UpdateField/DeleteField pass the same string to Mongo and to the published delta, so the boundary rejects instead of transforming. Seam: Service.gameRepo now depends on a narrow sceneStore port (Get/UpdateField/DeleteField) satisfied by *gameRepo.Store; NewService is unchanged. New internal/services/stage/stage_test.go is table-driven stdlib testing, no DB, no skips: a dotted sceneId is covered across all five entry points, plus dollar/leading-dollar/empty cases, assertions that a rejected id never reaches the store, that valid ids compose exactly the previous paths, and unit tables for validateSegment/validatePath. RED proof: with validation neutered the suite fails and shows the forged path 'stage.scenes.foo.privateData' being written and published. go build, go vet and go test -race ./... are green. Found while there: SetScript writes stage.scriptId, a bson path with no field on models.Stage, and LoadSceneFromScript writes 'stage.scene.<id>' (singular) where the model field is 'scenes' - both left untouched, to be filed separately.


### 2026-09-13T23:12:16.105Z - Proof completeness set PROVEN: 5 of 5 criteria; five entry points covered, three of which the story did not name, plus dollar, leading-dollar, leading-dot, trailing-dot and empty

### 2026-09-13T23:12:16.188Z - Proof feature-availability set PROVEN: Validation runs before any store call on every route that composes a path from a caller-supplied key

### 2026-09-13T23:12:16.274Z - Proof robustness set PROVEN: The recording fake asserts a rejected id never reaches the store, so the guard fails closed rather than rejecting after writing

### 2026-09-13T23:12:16.358Z - Proof resilience set NOT_APPLICABLE: Pure input validation with no failure path of its own

### 2026-09-13T23:12:16.439Z - Proof security set PROVEN: A dotted scene id can no longer forge a segment in the mongo update path or the published delta, which is the same defect 070 closed for events on a route escaping cannot reach

### 2026-09-13T23:12:16.526Z - Proof defense-in-depth set PROVEN: validateAndFetchGame covers SetCurrentScene as well, which needs no validation today but composes a path from the same caller-supplied id

### 2026-09-13T23:12:16.608Z - Proof input-validation set PROVEN: UpdateScene's optional sub-path is a dotted path by design, so it is split and each segment validated rather than rejecting dots outright and breaking legitimate nesting

### 2026-09-13T23:12:16.696Z - Proof thread-safety set NOT_APPLICABLE: Stateless validation over caller-supplied strings

### 2026-09-13T23:12:16.783Z - Proof configurability set NOT_APPLICABLE: Rejection rule is fixed, not tunable
