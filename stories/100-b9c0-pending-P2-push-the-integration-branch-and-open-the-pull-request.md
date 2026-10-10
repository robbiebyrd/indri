---
id: 100-b9c0
title: Push the integration branch and open the pull request
status: pending
priority: P2
type: task
created: "2026-10-10T01:14:21.000Z"
updated: "2026-10-10T01:14:21.000Z"
dependencies: ["099-a2fd"]
---

# Push the integration branch and open the pull request

## Problem Statement

Nothing on `POC-00003/main-integration` is pushed. All of this session's work (stories 062, 067, 090, 091, 093, 094; commits through 1d2d570) exists only locally, signed. CLAUDE.md forbids pushing or opening a PR unless asked, so this waits on Boss.

## Questions for Boss

1. Push before the rebase (story 099) as an off-machine backup, or only after it?
2. Which branch name goes up: the current one, or the rebased branch if 099 makes a new one?
3. One PR into `main`, or split it (e.g. the SQL backends, the transport, the Lua work) for reviewability? The diff is 557 files.
4. Draft or ready for review? Who reviews?
5. What goes in the description: a summary per story, the measured numbers (deltas 1926 → 273 bytes, keyframe 4614 → 725 bytes), and the open items (093 device checks, 042 blocked, 095–097)?
6. Should CI pass on the pushed branch before the PR is opened, or open it and let CI run on it?

## Acceptance Criteria

- [ ] Boss's answers recorded in the work log
- [ ] The branch is pushed and the PR opened as Boss chose, with its URL in the work log
- [ ] CI is green on the PR, or each failure is listed in the work log with its cause

## Work Log
