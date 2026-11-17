---
id: 095-8ce9
title: Decide how script kill counts reach an operator
status: wontfix
priority: P3
type: task
created: "2026-10-10T01:14:21.000Z"
updated: "2026-11-17T21:38:49.000Z"
dependencies: ["062-e7ab"]
---

# Decide how script kill counts reach an operator

## Problem Statement

Story 062 counts every killed Lua invocation by reason (`deadline`, `string_cap`, `depth_cap`, `node_cap`, `script_error`) in `Engine.KillCounts` and the process-wide `indri_lua_kills` expvar (`internal/services/lua/metrics.go`). Nothing serves that expvar over HTTP, so an operator cannot read the counts from a running server. The repo has no metrics library and no metrics endpoint today; it is the only expvar in the codebase.

## Questions for Boss

1. Should the counts be readable from a running server at all, or is in-process (`KillCounts`, tests) enough for now?
2. If served: plain `expvar` at `/debug/vars` (stdlib, no new dependency, JSON), or a metrics library (e.g. Prometheus client) with its own endpoint and format?
3. Which mux and port? The main HTTP mux serves players; a metrics route there is reachable by anyone who can reach the game. A separate listener (new env var, e.g. an `INDRI_` metrics address) keeps it off the public port.
4. Does the endpoint need authentication, or is network placement (separate port, not exposed) the control?
5. Should anything else be counted while we are at it (connections per transport, 4001/4002 closes, dropped frames from a full outbox), or only kills (YAGNI)?
6. Should kills also be logged per occurrence, with the action name and script path, or are counts enough?

## Acceptance Criteria

- [x] Boss's answers recorded in the work log
- [x] Kill counts readable the way Boss chose (or the story closed as wontfix if not needed), with a test that reads them over that path
- [REJECTED] .env.example and docs/SCRIPTING.md describe how to read them (not needed: nothing new to read; the counts stay in-process.)
- [REJECTED] VERIFY: go test -race ./... (not needed: no code change.)

## Work Log

### 2026-11-17T21:38:49.000Z - BOSS'S ANSWER (2026-11-17). Wontfix for now: in-process Engine.KillCounts and the indri_lua_kills expvar are enough until there is a real operator need. No endpoint, no new listener, nothing else counted. Reopen with this story's questions if an operator needs the numbers from a running server.
