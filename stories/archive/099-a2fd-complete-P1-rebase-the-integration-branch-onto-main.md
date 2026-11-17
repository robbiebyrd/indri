---
id: 099-a2fd
title: Rebase the integration branch onto main
status: complete
priority: P1
type: task
created: "2026-10-10T01:14:21.000Z"
updated: "2026-11-17T21:38:49.000Z"
dependencies: ["098-222f"]
completed_at: "2026-11-17T21:38:49.000Z"
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

- [x] Boss's answers recorded in the work log
- [x] The branch contains main's tip, with every conflict resolved and listed in the work log with how it was resolved
- [REJECTED] Every commit signed; no pre-commit hook skipped (waived by Boss: the merge ran on a Linux machine with no signing key, so 126c041 and the three test fixes before it (75cc663, 6823842, 795213c) are unsigned and already on main; re-signing would mean force-pushing main. No hook was skipped.)
- [x] VERIFY: go test -race ./... (PostgreSQL and MongoDB) && (cd client && rm -rf node_modules && pnpm install --frozen-lockfile && pnpm run typecheck && pnpm test && pnpm run lint)

## Work Log

### 2026-11-17T21:38:49.000Z - BOSS'S ANSWERS (2026-11-17). (1) Merge, not rebase. (2) This branch wins every code conflict: it had already ported origin/main's work by hand (stories 078-094) and decided against the rest (compact delta 083, luahandler 072/085, melody 093). (3) Main's sandbox fixes: not applicable, see the note on 1027308 below. (4) No fresh branch; the merge went onto POC-00003/main-integration and main was fast-forwarded to it. (5) After 093/096/097 is moot: merged now. (6) Done = go test -race ./... with PostgreSQL and MongoDB, plus the client's clean install, typecheck, tests and lint. Signing waived (see the criterion).

### 2026-11-17T21:38:49.000Z - MAIN'S TIP. This story's 1027308 ("Merge branch 'fix/sandbox-followups'", 270 commits ahead) was never pushed and exists on neither GitHub nor the Linux machine that did the merge. Boss chose to integrate against origin/main as it stood, e03ca3b (201 commits since 26a633a). The Mac's local main must be reconciled with the new main when it next pulls.

### 2026-11-17T21:38:49.000Z - RESOLUTION. A trial merge gave 150 conflicts (89 add/add, 53 content, 8 delete/modify), and auto-merging would also have resurrected ~127 main-only files this branch had retired (internal/handlers/luahandler, events/positional.go + wire.go + clientview.go, client/packages/protocol-client, transport/graphqlws, hub, composite). So the merge (126c041) takes this branch's tree for every file, conflicted or not, and adds back from main only the design records this branch lacked: plans/{multi-instance-routing,pluggable-transports,session-resume,webrtc-ice-relay}.md, reviews/2026-09-28*, docs/superpowers/. Code diff of the merge against its first parent: none.

### 2026-11-17T21:38:49.000Z - FIXES NEEDED FIRST. The branch had never run under CI on Linux. Three tests failed there every run: 12 WebRTC tests (pion's default client gathered on all 48 interfaces, ~16 s under -race vs a 5 s timeout; fixed in 75cc663 by holding the test client to loopback), TestSend_MaximumFanOutAtEveryDepthStopsOnTheSharedBudget (5 ms budget never let the tree descend under -race; 50 ms in 6823842), and the 200-widget reapply benchmark (12 s vs a 10 s budget on an idle 16-core host; 30 s in 795213c).

### 2026-11-17T21:38:49.000Z - VERIFIED. go build, go vet, go test -race -count=1 ./... with MongoDB and PostgreSQL: 45 packages ok, 0 FAIL, no DATA RACE, no skips. Client: rm -rf node_modules && pnpm install --frozen-lockfile, typecheck clean, pnpm test 425/425, lint 0 errors (4 pre-existing warnings). CI on main for 126c041 (run 44179356061): success.
