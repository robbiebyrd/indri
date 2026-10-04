---
id: 094-7d2e
title: Send the layout once per connection instead of in every keyframe
status: pending
priority: P2
type: feature
created: "2026-10-04T18:33:52.000Z"
updated: "2026-10-04T18:33:52.000Z"
dependencies: ["083-40ee"]
---

# Send the layout once per connection instead of in every keyframe

## Problem Statement

Story 083 measured the tictactoe keyframe at 4936 bytes, 3159 of them the layout, re-sent on every join, refresh and reconnect. Sending the layout on its own content-hash-versioned frame once per connection takes the keyframe to 754 bytes. Boss chose this alongside compression (083). It is orthogonal to the delta encoding.

## Acceptance Criteria

- [ ] A keyframe carries the layout's content hash, not the layout, once the connection holds that version
- [ ] A connection is sent the layout frame the first time it needs a given hash, and again only when the hash changes
- [ ] Layout edits (the layout action) still reach every player, as deltas or a new layout frame, and the client renders identically
- [ ] SanitizeDelta and GameService.Sanitize still agree, and privateData stays reserved inside a layout
- [ ] Measured keyframe size recorded before and after on the tictactoe config
- [ ] VERIFY: go test -race ./... && (cd client && pnpm run typecheck && pnpm test)

## Work Log
