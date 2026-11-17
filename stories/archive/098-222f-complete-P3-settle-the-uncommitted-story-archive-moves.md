---
id: 098-222f
title: Settle the uncommitted story archive moves
status: complete
priority: P3
type: chore
created: "2026-10-10T01:14:21.000Z"
updated: "2026-11-17T21:38:49.000Z"
dependencies: []
completed_at: "2026-11-17T21:38:49.000Z"
---

# Settle the uncommitted story archive moves

## Problem Statement

The working tree holds six story moves from `stories/` to `stories/archive/` that nobody committed: 078-ceca, 079-24a1, 080-cf52, 081-7e0a, 082-b854 and 088-69ac, all `complete`. They were there before this session's work started. Git shows them as deletions plus untracked files, so a careless `git add -A` would sweep them into an unrelated commit. Other complete stories (083–092, 094) are still in `stories/`, so there is no consistent rule for what gets archived.

## Questions for Boss

1. Did you (or another session) make these moves on purpose? Commit them, or restore the files to `stories/`?
2. Is there a rule for archiving: every `complete`/`wontfix` story, stories older than some date, or only by hand?
3. If there is a rule, should this story apply it to the rest (068–077 wontfix, 083–092 and 094 complete) in the same commit?
4. Should CLAUDE.md's Stories section document `stories/archive/`?

## Acceptance Criteria

- [x] Boss's answers recorded in the work log
- [x] `git status` shows no stray story moves: they are committed (as `git mv` renames) or restored
- [x] If a rule was chosen, it is applied and written in CLAUDE.md

## Work Log

### 2026-11-17T21:38:49.000Z - BOSS'S ANSWERS (2026-11-17). (1) The moves were made on purpose: Boss's commit 2804fc1 ("archive stories") committed them as pure renames, along with every other complete story. (2) Rule: a story is archived when it is closed, complete or wontfix. (3) Applied in the same commit: 068-077 (wontfix) and the stories closed today (095, 097, 098, 099, 100) moved to stories/archive/. (4) Documented in CLAUDE.md's Stories section.
