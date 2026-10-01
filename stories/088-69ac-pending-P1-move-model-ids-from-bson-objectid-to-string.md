---
id: 088-69ac
title: Move model IDs from bson.ObjectID to string
status: pending
priority: P1
type: refactor
created: "2026-10-01T14:32:29.318Z"
updated: "2026-10-01T14:32:33.065Z"
dependencies: ["079-24a1"]
plan: plans/main-divergence-integration.md
plan_step: Step 3
depends_on: ["stories/079-24a1-in_progress-P1-port-the-sqlite-and-postgres-clients-and-the-repo-.md"]
---

# Move model IDs from bson.ObjectID to string

## Problem Statement

models.Game, models.User and models.Session all declare ID as bson.ObjectID (internal/models/game.go:10, user.go:10, session.go:10). The Storer interfaces already pass ids as string, but the stored type is Mongo-specific, so a SQLite or Postgres store cannot produce one and the ported DDL ('id TEXT PRIMARY KEY', ids.New() returning a UUID) has nothing to write into. origin/main solved this in 0b7a246. This is a cross-cutting prerequisite for every non-Mongo backend and is too large to sit inside the 'add two game backends' story.

## Acceptance Criteria

- [ ] models.Game.ID, models.User.ID and models.Session.ID are string, not bson.ObjectID
- [ ] The Mongo stores convert at the driver boundary so existing documents still read and write correctly
- [ ] New ids come from repo/ids.New(); no code constructs a bson.ObjectID to mint an identifier
- [ ] The connection key sessionId still carries session.ID and is still never sent to clients, while the wire field sessionId still carries session.Token
- [ ] Every handler that builds or compares an id works on strings, with no ObjectID parsing left outside internal/repo
- [ ] Existing game, user and session store tests pass unchanged in behaviour
- [ ] VERIFY: go build ./... && go vet ./... && go test -race ./internal/repo/... ./internal/handlers/...
- [ ] Full suite shows no regression against .integration/baseline.txt

## Files

- internal/models/game.go
- internal/models/user.go
- internal/models/session.go
- internal/repo/game/
- internal/repo/user/
- internal/repo/session/

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

