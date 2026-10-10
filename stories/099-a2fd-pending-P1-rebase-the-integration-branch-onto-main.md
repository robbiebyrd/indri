---
id: 099-a2fd
title: Rebase the integration branch onto main
status: pending
priority: P1
type: task
created: "2026-10-10T01:14:21.000Z"
updated: "2026-10-10T01:14:21.000Z"
dependencies: ["098-222f"]
---

# Rebase the integration branch onto main

## Problem Statement

`POC-00003/main-integration` is 133 commits ahead of `main` and 270 behind (merge base 26a633a; main is at 1027308, "Merge branch 'fix/sandbox-followups'"). The branch diff against main touches 557 files (+90481/−3599). Boss said earlier "keep going here, we'll rebase with main later". Known collision areas:

- The delta wire format: main uses the compact protocol (positional paths over MessagePack). This branch declined it (story 083) and keeps JSON with dotted paths, plus WebSocket compression.
- The WebSocket transport: main is on melody/gorilla. This branch rewrote it on coder/websocket (story 093), adding close codes 4001/4002 and the login deadline.
- The keyframe shape: `layoutHash` and a layout-less keyframe (story 094).
- The origin policy (story 086, and 097 if it changes it).
- Whatever main's 270 commits changed in the Lua sandbox (its tip merges "host calls charged against the script budget, Lua lists as arrays, per-key override drops, script layouts validated at boot"), which overlaps this branch's Lua work (062, 085).

## Questions for Boss

1. Rebase or merge? A rebase of 133 signed commits re-signs every one (one YubiKey touch each unless the agent caches) and rewrites the branch's history. A merge commit keeps it and is one conflict resolution pass.
2. Where main and this branch made opposite decisions (compact protocol, melody), does this branch win by default, given the 083 and 093 decisions? Or does each conflict come back to you?
3. Main's sandbox fixes ("Lua lists as arrays", host calls charged against the budget): adopt main's behaviour where it conflicts with this branch's, or decide case by case?
4. Should the rebase or merge happen on a fresh branch (e.g. `POC-00003/main-integration-rebased`) so the current branch stays as a fallback?
5. Before or after stories 093 (device checks), 096 and 097 land? Landing them first means more to rebase. Rebasing first means they are built on main's code.
6. The definition of done: the full `go test -race ./...` with PostgreSQL and MongoDB, `pnpm test`, typecheck and lint, plus a live tictactoe game in a browser? Anything more?

## Acceptance Criteria

- [ ] Boss's answers recorded in the work log
- [ ] The branch contains main's tip, with every conflict resolved and listed in the work log with how it was resolved
- [ ] Every commit signed; no pre-commit hook skipped
- [ ] VERIFY: go test -race ./... (PostgreSQL and MongoDB) && (cd client && rm -rf node_modules && pnpm install --frozen-lockfile && pnpm run typecheck && pnpm test && pnpm run lint)

## Work Log
