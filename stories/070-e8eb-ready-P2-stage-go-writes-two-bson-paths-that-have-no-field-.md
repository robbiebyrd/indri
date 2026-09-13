---
id: 070-e8eb
title: stage.go writes two bson paths that have no field on the model
status: ready
priority: P2
type: fix
created: "2026-09-13T23:12:28.181Z"
updated: "2026-09-13T23:19:58.468Z"
dependencies: []
---

# stage.go writes two bson paths that have no field on the model

## Problem Statement

Two writes in internal/services/stage/stage.go target bson paths with no corresponding field on models.Stage, so they store data nothing reads and publish deltas the client cannot use. LoadSceneFromScript at line 222 builds stage.scene. singular, while every other site in the file builds stage.scenes. plural; the model bson tag is scenes, so a scene loaded from the script is written into a field that does not exist. Separately SetScript at line 126 writes stage.scriptId, and grep finds no scriptId anywhere else in the repo and no such field on models.Stage - an earlier review flagged it as dead or aspirational. Both were found while implementing 069-f93a and deliberately left alone there. stage.Service is still not wired into the injector, so neither is reachable yet.

## Acceptance Criteria

- [ ] LoadSceneFromScript writes to the same stage.scenes path every other method in the file uses
- [ ] A test proves a scene loaded from the script is readable back through the model, which the singular path would fail
- [ ] SetScript is either removed, or given a real field on models.Stage plus a reader, with the choice and its reason recorded
- [ ] No write in the package targets a bson path that has no corresponding model field
- [ ] VERIFY: go test -race ./internal/services/stage/

## Files

- internal/services/stage/stage.go
- internal/models/stage.go

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

