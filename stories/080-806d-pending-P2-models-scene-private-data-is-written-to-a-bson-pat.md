---
id: "080-806d"
title: "models.Scene private data is written to a bson path the model does not declare"
status: pending
priority: P2
type: fix
created: 2026-09-13T23:28:36.153Z
updated: 2026-09-13T23:28:36.153Z
dependencies: []
---

# models.Scene private data is written to a bson path the model does not declare

## Problem Statement

models.Scene.PrivateData is tagged bson private_data - the only snake_case bson tag in the repo - while models.DataStorePrivate is privateData. So UpdateScene and LoadSceneFromScript with DataStorePrivate write stage.scenes.<id>.privateData, a field models.Scene does not declare: stored, never readable back through the struct, and the delta names a path with nothing behind it. This is the third instance of the defect class found in 070-e8eb, after the singular stage.scene path and the phantom stage.scriptId, and the generic path test added there excludes this one case with a comment saying why. There is no data leak: the delta path uses json names so SanitizeDelta still drops it, and keyframes are built from the struct. The fix is not free - changing the tag changes the persisted format for scenes already stamped from config.json, so it needs a migration decision rather than a one-character edit.

## Acceptance Criteria

- [ ] A decision is recorded on whether to change the bson tag to privateData or to change DataStorePrivate, with the persisted-data consequence stated
- [ ] Scene private data written through DataStorePrivate is readable back through models.Scene
- [ ] The exclusion in TestEveryPathThisPackageWritesNamesAFieldTheModelsDeclare is removed, so the generic guard covers every path with no exceptions
- [ ] If the persisted format changes, the effect on games already stored is stated and handled
- [ ] VERIFY: go test -race ./internal/services/stage/ ./internal/models/

## Files

- internal/models/scene.go
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

