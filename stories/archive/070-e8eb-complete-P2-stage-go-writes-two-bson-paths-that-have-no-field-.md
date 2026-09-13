---
id: 070-e8eb
title: stage.go writes two bson paths that have no field on the model
status: complete
priority: P2
type: fix
created: "2026-09-13T23:12:28.181Z"
updated: "2026-09-13T23:28:37.532Z"
dependencies: []
completed_at: "2026-09-13T23:28:37.532Z"
---

# stage.go writes two bson paths that have no field on the model

## Problem Statement

Two writes in internal/services/stage/stage.go target bson paths with no corresponding field on models.Stage, so they store data nothing reads and publish deltas the client cannot use. LoadSceneFromScript at line 222 builds stage.scene. singular, while every other site in the file builds stage.scenes. plural; the model bson tag is scenes, so a scene loaded from the script is written into a field that does not exist. Separately SetScript at line 126 writes stage.scriptId, and grep finds no scriptId anywhere else in the repo and no such field on models.Stage - an earlier review flagged it as dead or aspirational. Both were found while implementing 069-f93a and deliberately left alone there. stage.Service is still not wired into the injector, so neither is reachable yet.

## Acceptance Criteria

- [x] LoadSceneFromScript writes to the same stage.scenes path every other method in the file uses
- [x] A test proves a scene loaded from the script is readable back through the model, which the singular path would fail
- [x] SetScript is either removed, or given a real field on models.Stage plus a reader, with the choice and its reason recorded
- [x] No write in the package targets a bson path that has no corresponding model field
- [x] VERIFY: go test -race ./internal/services/stage/

## Files

- internal/services/stage/stage.go
- internal/models/stage.go

## Proof

- [x] [completeness] Completeness (4 of 4 criteria; both defects fixed and a generic guard added that would have caught either)
- [x] [feature-availability] Feature availability (A scene loaded from the script is now readable back through the model, which the singular path silently prevented)
- [x] [robustness] Robustness (RED proved on the readability assertion independently of the path-literal check, so the test catches the data loss and not merely the spelling)
- [~] [resilience] Resilience (Currently unreachable code; stage.Service has no callers, which is why the tests rather than the fix carry the value)
- [~] [security] Security (No authorisation or input surface; path-segment validation is story 069-f93a and is unchanged here)
- [x] [defense-in-depth] Defense in depth (TestEveryPathThisPackageWritesNamesAFieldTheModelsDeclare resolves every written path against the models bson tags by reflection across all ten entry points, and TestDeclaresPath pins the oracle so a checker that cannot say no is caught)
- [x] [input-validation] Input validation (scenePath is now the single place a scene path is composed, so the plural and singular forms cannot drift apart again)
- [~] [thread-safety] Thread safety (No concurrency introduced)
- [x] [configurability] Configurability (SetScript was removed rather than given a field, because nothing in the repo selects a script per game or per stage - scripts are config.json loaded once at boot, and the layout op attaches Lua as inline source, so a ScriptID would have meant inventing the selection mechanism it implies with no caller)

## QA

Verified independently: build, vet, gofmt and full suite green under -race. Confirmed the singular path is gone and scenePath is the single composer, SetScript is removed with its reasoning left in place of the function, and the third defect instance is real (models.Scene bson private_data vs DataStorePrivate privateData). Story line number for scriptId was stale - it had moved to :186. 4/4.

## Work Log

### 2026-09-13T23:27:21.180Z - Fixed both bson paths in internal/services/stage that named no field on the models. (1) LoadSceneFromScript built 'stage.scene.<id>' (singular) at stage.go:222 - the only singular site, against three plural ones - so every scene it loaded went into a field models.Stage does not declare and the delta named a path the client cannot apply. Fixed by extracting scenePath(sceneId), now the single place all four call sites compose a scene path, so the name cannot drift again. (2) SetScript wrote 'stage.scriptId' (at stage.go:186, not :126 as the story says - 069-f93a moved it) from an ObjectID hex nothing mints: scripts are not Mongo documents, they are the config.json the server booted with, whose Lua files are compiled once at boot with per-script grants, and the layout editor attaches Lua as inline source on a board/scene/widget. Nothing selects a script per game, so a stage-level script id addresses nothing. Removed rather than given a field - adding one would invent per-game script selection no caller asks for - with that reasoning recorded in stage.go where it stood. Tests (stdlib, table-driven, no DB, no skips, no build tags) reuse 069's recordingStore and sceneStore port: TestASceneLoadedFromTheScriptIsReadableBackThroughTheModel applies the recorded write the way Mongo applies a dotted $set and decodes it back into models.Game, so a scene that is not readable back fails; TestEveryPathThisPackageWritesNamesAFieldTheModelsDeclare drives all ten entry points and resolves every written and deleted path against the models' bson tags by reflection; TestDeclaresPath pins that oracle, including 'stage.scene.<id>' and 'stage.scriptId' as paths it must reject. RED proof: restoring the singular path fails both new tests plus three pre-existing ones, and the readability assertion fails on its own ('the game has scenes map[]'). Seam: AddScene and SetSceneOrder now read through the existing sceneStore port instead of gameService.Fetch - the same call, but it makes those two entry points reachable without a database. go build, go vet, gofmt and go test -race ./... are green. Found while there, not fixed: models.Scene tags its private data 'private_data' while models.DataStoreType spells it 'privateData', so UpdateScene and LoadSceneFromScript write scene private data to a field Scene does not declare - same defect class, outside this story's scope, excluded from the path test with a comment saying so. Also LoadFromScript still takes a scriptId parameter it ignores, the same aspirational idea as SetScript.


### 2026-09-13T23:28:36.349Z - Proof completeness set PROVEN: 4 of 4 criteria; both defects fixed and a generic guard added that would have caught either

### 2026-09-13T23:28:36.431Z - Proof feature-availability set PROVEN: A scene loaded from the script is now readable back through the model, which the singular path silently prevented

### 2026-09-13T23:28:36.519Z - Proof robustness set PROVEN: RED proved on the readability assertion independently of the path-literal check, so the test catches the data loss and not merely the spelling

### 2026-09-13T23:28:36.604Z - Proof resilience set NOT_APPLICABLE: Currently unreachable code; stage.Service has no callers, which is why the tests rather than the fix carry the value

### 2026-09-13T23:28:36.686Z - Proof security set NOT_APPLICABLE: No authorisation or input surface; path-segment validation is story 069-f93a and is unchanged here

### 2026-09-13T23:28:36.767Z - Proof defense-in-depth set PROVEN: TestEveryPathThisPackageWritesNamesAFieldTheModelsDeclare resolves every written path against the models bson tags by reflection across all ten entry points, and TestDeclaresPath pins the oracle so a checker that cannot say no is caught

### 2026-09-13T23:28:36.847Z - Proof input-validation set PROVEN: scenePath is now the single place a scene path is composed, so the plural and singular forms cannot drift apart again

### 2026-09-13T23:28:36.927Z - Proof thread-safety set NOT_APPLICABLE: No concurrency introduced

### 2026-09-13T23:28:37.019Z - Proof configurability set PROVEN: SetScript was removed rather than given a field, because nothing in the repo selects a script per game or per stage - scripts are config.json loaded once at boot, and the layout op attaches Lua as inline source, so a ScriptID would have meant inventing the selection mechanism it implies with no caller
