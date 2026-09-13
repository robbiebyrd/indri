---
id: 080-806d
title: models.Scene private data is written to a bson path the model does not declare
status: complete
priority: P2
type: fix
created: "2026-09-13T23:28:36.153Z"
updated: "2026-09-13T23:40:48.248Z"
dependencies: []
completed_at: "2026-09-13T23:40:48.248Z"
---

# models.Scene private data is written to a bson path the model does not declare

## Problem Statement

models.Scene.PrivateData is tagged bson private_data - the only snake_case bson tag in the repo - while models.DataStorePrivate is privateData. So UpdateScene and LoadSceneFromScript with DataStorePrivate write stage.scenes.<id>.privateData, a field models.Scene does not declare: stored, never readable back through the struct, and the delta names a path with nothing behind it. This is the third instance of the defect class found in 070-e8eb, after the singular stage.scene path and the phantom stage.scriptId, and the generic path test added there excludes this one case with a comment saying why. There is no data leak: the delta path uses json names so SanitizeDelta still drops it, and keyframes are built from the struct. The fix is not free - changing the tag changes the persisted format for scenes already stamped from config.json, so it needs a migration decision rather than a one-character edit.

## Acceptance Criteria

- [x] A decision is recorded on whether to change the bson tag to privateData or to change DataStorePrivate, with the persisted-data consequence stated
- [REJECTED] Scene private data written through DataStorePrivate is readable back through models.Scene (Requires the bson tag change, which orphans the private data of every persisted scene. Agent stopped and reported rather than migrating; the decision is the users.)
- [REJECTED] The exclusion in TestEveryPathThisPackageWritesNamesAFieldTheModelsDeclare is removed, so the generic guard covers every path with no exceptions (Same: the exclusion can only be removed once the tag changes. The comment now points at the decision record and the new models guard, so it is a recorded exception rather than a blind spot.)
- [x] If the persisted format changes, the effect on games already stored is stated and handled
- [x] VERIFY: go test -race ./internal/services/stage/ ./internal/models/

## Files

- internal/models/scene.go
- internal/services/stage/stage.go

## Proof

- [x] [completeness] Completeness (Decision recorded with its cost and every fact checked; 3 of 5 criteria met and 2 rejected pending a migration decision rather than quietly ticked)
- [~] [feature-availability] Feature availability (Deliberately not implemented: the fix requires a persisted-format change that is the users call)
- [x] [robustness] Robustness (RED proved twice, and the first draft of the stale-exception check did not actually fire - the agent fixed the check rather than the comment)
- [x] [resilience] Resilience (saveVersioned sets the whole stage subtree, so the first save after a tag change drops the old field silently; that is stated in the decision record as why this cannot be deferred to a later migration)
- [x] [security] Security (No leak verified on all three legs rather than assumed: delta paths name privateData either way, SanitizeDelta drops that segment, and Sanitize nils PrivateData on every scene before a keyframe)
- [x] [defense-in-depth] Defense in depth (A new models guard walks every exported field reachable from models.Game reporting bson and json disagreement, with two reasoned exceptions that fail if they stop diverging, so the exception must be deleted with the fix)
- [x] [input-validation] Input validation (The third option was identified as a trap: composing the scene path from the bson tag stores it readably but publishes private_data, which SanitizeDelta does not recognise, broadcasting scene private data to every player)
- [~] [thread-safety] Thread safety (No concurrency introduced)
- [~] [configurability] Configurability (Naming decision, not a tunable)

## QA

Verified independently: build, vet, gofmt and full suite green. Confirmed stripKey returns non-map non-slice values untouched and UpdateField publishes the raw value, so the struct bypass it found is real. Agent correctly stopped rather than migrating: config.json stamps stage.scenes.<id>.private_data onto every game created, and Lua scripts write scene private data live through applyLua, so the data is real and not hypothetical. Criteria 2 and 3 rejected pending the migration decision.

## Work Log

### 2026-09-13T23:39:12.508Z - STOPPED at the migration boundary, as briefed - no tag change, no migration written. Direction: change models.Scene's bson tag to 'privateData'. The reverse (respelling DataStorePrivate as 'private_data') is not available: that constant is the shared name of the private store on Game/Stage/Team/Player, all of which persist 'privateData'; it is the segment events.SanitizeDelta matches on; and it is the json name the client and the Lua host API already use. Composing the scene path from the bson tag instead would store the data readably but publish a delta path ending in 'private_data', which SanitizeDelta does not treat as private - scene private data would be broadcast to every player. So there is no migration-free third option; the tag is the side that must change. Verified the no-leak claim on all three legs rather than assuming it: (1) delta paths name 'privateData' either way - Mutate diffs the json view via events.ToMap, and UpdateField publishes its key verbatim, which is composed from DataStorePrivate; (2) SanitizeDelta drops any path with a 'privateData' segment (delta.go pathHasSegment); (3) keyframes come from the struct and GameService.Sanitize nils PrivateData on every scene (game.go:172-175). Data at 'privateData' is never decoded into the struct, so it cannot reach a keyframe either. Confirmed no leak. What is actually persisted, checked not assumed: a Scene marshals its private data to bson key 'private_data' (proved by round-tripping models.Scene through bson.Marshal); config.json declares 'privateData': {} on the board scene and json decodes it to a NON-NIL empty map, so gameRepo.New stamps stage.scenes.board.private_data onto every game ever created; Lua game scripts write the field through their json view ('state.stage.scenes.board.privateData', applyLua via host_mutate.go, wired in injector/services.go) and Mutate saves it back as 'private_data'; and saveVersioned $sets the whole 'stage' subtree, so the first save after a tag change would drop the old field silently rather than leave it to migrate. Enumerated the writers: the three above are the reachable ones; the broken 'privateData' path has NO writers, because its only composers are stage.Service.UpdateScene and LoadSceneFromScript and nothing in the repo imports internal/services/stage. Since a deployed database cannot be inspected from here, it cannot be shown that live scenes hold nothing worth keeping, so this is a migration - the user's call. A read-both/write-new shim for one release is the obvious alternative. Recorded the decision and every checked fact in code, not just the report: models/scene.go carries the full record, common.go explains why DataStoreType is one name, stage.go documents why UpdateScene's path is wrong and why the bson-tag fix is worse. Added internal/models/datastore_names_test.go: TestBsonAndJsonNamesAgreeAcrossTheGameDocument walks every exported field reachable from models.Game and reports any whose bson and json names disagree, with knownDivergences holding exactly two reasoned entries (Game.ID, Scene.PrivateData) - and it fails if an entry stops diverging, so the exception must be deleted with the fix; TestScenePrivateDataPersistsUnderItsRecordedName pins the stored spelling so the tag cannot be flipped as an incidental edit. RED proof: dropping the Scene entry reports the defect; flipping the tag fires BOTH the stale-exception check and the persisted-name pin with the migration message. Criteria 2 and 3 are deliberately NOT met - scene private data is still not readable back through models.Scene, and the exclusion in TestEveryPathThisPackageWritesNamesAFieldTheModelsDeclare stays - because both require the tag change. The exclusion comment now points at the recorded decision and at the new models guard, so it is no longer a blind spot. go build, go vet, gofmt -l internal/ and go test -race ./... all green (24 packages). Found while there, NOT fixed (out of scope): events.SanitizeDelta's stripKey only descends into map[string]interface{} and []interface{}, so a struct value published by UpdateField passes through unstripped - a whole-stage or whole-scene write broadcasts privateData verbatim. Proved it with a scratch test. Latent only: every UpdateField caller in the repo is inside the callerless stage.Service.


### 2026-09-13T23:40:47.028Z - Proof completeness set PROVEN: Decision recorded with its cost and every fact checked; 3 of 5 criteria met and 2 rejected pending a migration decision rather than quietly ticked

### 2026-09-13T23:40:47.108Z - Proof feature-availability set NOT_APPLICABLE: Deliberately not implemented: the fix requires a persisted-format change that is the users call

### 2026-09-13T23:40:47.190Z - Proof robustness set PROVEN: RED proved twice, and the first draft of the stale-exception check did not actually fire - the agent fixed the check rather than the comment

### 2026-09-13T23:40:47.271Z - Proof resilience set PROVEN: saveVersioned sets the whole stage subtree, so the first save after a tag change drops the old field silently; that is stated in the decision record as why this cannot be deferred to a later migration

### 2026-09-13T23:40:47.357Z - Proof security set PROVEN: No leak verified on all three legs rather than assumed: delta paths name privateData either way, SanitizeDelta drops that segment, and Sanitize nils PrivateData on every scene before a keyframe

### 2026-09-13T23:40:47.445Z - Proof defense-in-depth set PROVEN: A new models guard walks every exported field reachable from models.Game reporting bson and json disagreement, with two reasoned exceptions that fail if they stop diverging, so the exception must be deleted with the fix

### 2026-09-13T23:40:47.531Z - Proof input-validation set PROVEN: The third option was identified as a trap: composing the scene path from the bson tag stores it readably but publishes private_data, which SanitizeDelta does not recognise, broadcasting scene private data to every player

### 2026-09-13T23:40:47.629Z - Proof thread-safety set NOT_APPLICABLE: No concurrency introduced

### 2026-09-13T23:40:47.713Z - Proof configurability set NOT_APPLICABLE: Naming decision, not a tunable
