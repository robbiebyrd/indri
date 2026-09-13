---
id: 082-5814
title: Flip Scene.PrivateData to the privateData bson tag
status: complete
priority: P2
type: fix
created: "2026-09-13T23:42:35.379Z"
updated: "2026-09-13T23:49:50.770Z"
dependencies: []
completed_at: "2026-09-13T23:49:50.770Z"
---

# Flip Scene.PrivateData to the privateData bson tag

## Problem Statement

Story 080-806d established that models.Scene.PrivateData is tagged bson private_data while it is addressed everywhere else as privateData, so scene private data is stored where the struct cannot read it back. The fix was held because it orphans the private data of every persisted scene. Robbie has now decided: accept the one-time loss, there is no real data to speak of. This story executes that decision. Two guards added by 080-806d are designed to fail until it is done - knownDivergences in the models test fails if its Scene.PrivateData entry stops diverging, and the stage package path test excludes this one case - so both must be updated as part of the change rather than after it.

## Acceptance Criteria

- [x] models.Scene.PrivateData is tagged bson privateData, matching every other private store in the game document
- [x] Scene private data written through DataStorePrivate is readable back through models.Scene
- [x] The Scene.PrivateData entry is removed from knownDivergences and the models guard passes with only the Game.ID exception remaining
- [x] The exclusion in TestEveryPathThisPackageWritesNamesAFieldTheModelsDeclare is removed and that guard passes with no exceptions
- [x] The accepted one-time loss is recorded in the code with who decided it and why it was acceptable, replacing the decision record that said it was pending
- [x] Nothing else in the game document changes spelling
- [x] VERIFY: go test -race ./internal/models/ ./internal/services/stage/ ./internal/services/events/

## Files

- internal/models/scene.go
- internal/models/datastore_names_test.go
- internal/services/stage/stage_test.go

## Proof

- [x] [completeness] Completeness (7 of 7 criteria; tag flipped, both guards updated, decision rewritten, readability proved)
- [x] [feature-availability] Feature availability (Scene private data written through DataStorePrivate is now readable back through models.Scene, pinned by its own subtest)
- [x] [robustness] Robustness (Each guard was checked separately rather than assumed: removing the stage exclusion on the old tag failed all three private-store calls, and flipping the tag fired both the stale-entry check and the persisted-name pin)
- [x] [resilience] Resilience (The pin now asserts against DataStorePrivate.String（） rather than a string literal, so the tag and the constant stage composes its path from cannot drift apart silently again)
- [x] [security] Security (Scene private data is now stored where Sanitize can nil it on the struct, closing the case where data at privateData was never decoded into the model at all)
- [x] [defense-in-depth] Defense in depth (The models guard passes with Game.ID as the only exception and the stage path guard passes with none, so neither carries a standing excuse any more)
- [x] [input-validation] Input validation (config.json and example config declare privateData at four sites each and contain no private_data; confirmed by grep rather than assumed, since a json name is unaffected by a bson tag change)
- [~] [thread-safety] Thread safety (Struct tag change with no concurrency implications)
- [~] [configurability] Configurability (Naming correction, not a tunable)

## QA

Verified independently: models and stage packages green under -race; the diff changes exactly one bson tag; knownDivergences is down to Game.ID alone; every remaining private_data occurrence is a comment recording the history. Both guards proved real - the agent removed the stage exclusion while still on the old tag and watched all three private-store calls fail, and flipping the tag fired both halves of the models guard. 7/7.

## Work Log

### 2026-09-13T23:48:46.502Z - Flipped models.Scene.PrivateData from bson private_data to privateData - one tag line, the only spelling change in the whole game document (verified by grepping the diff for bson:/json: lines). Both guards from 080-806d were real and both failed before the change, checked separately rather than assumed. Guard 2 first, still on the old tag: adding the three excluded private-store calls to TestEveryPathThisPackageWritesNamesAFieldTheModelsDeclare (UpdateScene, UpdateScene with a sub-path, LoadSceneFromScript) failed on all three with 'stage.scenes.round1.privateData names no field on the models'. Then the tag flip fired both halves of the models guard: TestBsonAndJsonNamesAgreeAcrossTheGameDocument reported the now-stale knownDivergences entry, and TestScenePrivateDataPersistsUnderItsRecordedName fired its migration message. Removed the Scene.PrivateData entry, leaving Game.ID as the only exception; removed the stage exclusion paragraph so that guard now drives every data store of every entry point with no exceptions. Updated TestScenePrivateDataPersistsUnderItsRecordedName rather than deleting it: it now pins the spelling against models.DataStorePrivate.String() instead of a literal, so the pin also asserts the tag matches the constant stage composes its update path from - the two can no longer drift apart silently. Criterion 2 proof: added 'only the private data store' to TestASceneLoadedFromTheScriptIsReadableBackThroughTheModel, which applies the recorded write the way Mongo applies a dotted $set and decodes it back into models.Game; extracted a readsBack helper and strengthened the whole-scene case to assert both stores. RED proof that it discriminates: reverting the tag alone fails exactly that subtest ('read back without its private data'). Rewrote the decision record on models.Scene - it shrank from 71 to 53 lines because it no longer describes a pending decision. It now records that Robbie decided to accept the one-time loss (no deployment holds scene private data worth keeping) rather than write a migration or carry a read-both/write-new shim indefinitely for data known to be empty, and keeps the analysis of why the other two directions were wrong, since that is the justification for this direction. Updated the two stale pointers to it: common.go said the names diverge except on Scene, and stage.go's UpdateScene said the fix was blocked on a migration decision. Confirmed rather than assumed: config.json and example/tictactoe/config.json declare privateData (json name, 4 sites each) and contain no private_data at all, so a bson tag change cannot affect them - neither file needed editing. go build, go vet, gofmt -l internal/ and go test -race ./... all green; nothing outside internal/models and internal/services/stage was touched.


### 2026-09-13T23:49:47.428Z - Proof completeness set PROVEN: 7 of 7 criteria; tag flipped, both guards updated, decision rewritten, readability proved

### 2026-09-13T23:49:47.513Z - Proof feature-availability set PROVEN: Scene private data written through DataStorePrivate is now readable back through models.Scene, pinned by its own subtest

### 2026-09-13T23:49:47.597Z - Proof robustness set PROVEN: Each guard was checked separately rather than assumed: removing the stage exclusion on the old tag failed all three private-store calls, and flipping the tag fired both the stale-entry check and the persisted-name pin

### 2026-09-13T23:49:47.696Z - Proof resilience set PROVEN: The pin now asserts against DataStorePrivate.String() rather than a string literal, so the tag and the constant stage composes its path from cannot drift apart silently again

### 2026-09-13T23:49:47.796Z - Proof security set PROVEN: Scene private data is now stored where Sanitize can nil it on the struct, closing the case where data at privateData was never decoded into the model at all

### 2026-09-13T23:49:47.877Z - Proof defense-in-depth set PROVEN: The models guard passes with Game.ID as the only exception and the stage path guard passes with none, so neither carries a standing excuse any more

### 2026-09-13T23:49:47.959Z - Proof input-validation set PROVEN: config.json and example config declare privateData at four sites each and contain no private_data; confirmed by grep rather than assumed, since a json name is unaffected by a bson tag change

### 2026-09-13T23:49:48.043Z - Proof thread-safety set NOT_APPLICABLE: Struct tag change with no concurrency implications

### 2026-09-13T23:49:48.124Z - Proof configurability set NOT_APPLICABLE: Naming correction, not a tunable
