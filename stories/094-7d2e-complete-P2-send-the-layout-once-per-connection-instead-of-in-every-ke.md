---
id: 094-7d2e
title: Send the layout once per connection instead of in every keyframe
status: complete
priority: P2
type: feature
created: "2026-10-04T18:33:52.000Z"
updated: "2026-10-08T23:15:52.000Z"
dependencies: ["083-40ee"]
---

# Send the layout once per connection instead of in every keyframe

## Problem Statement

Story 083 measured the tictactoe keyframe at 4936 bytes, 3159 of them the layout, re-sent on every join, refresh and reconnect. Sending the layout on its own content-hash-versioned frame once per connection takes the keyframe to 754 bytes. Boss chose this alongside compression (083). It is orthogonal to the delta encoding.

## Acceptance Criteria

- [x] A keyframe carries the layout's content hash, not the layout, once the connection holds that version
- [x] A connection is sent the layout frame the first time it needs a given hash, and again only when the hash changes
- [x] Layout edits (the layout action) still reach every player, as deltas or a new layout frame, and the client renders identically
- [x] SanitizeDelta and GameService.Sanitize still agree, and privateData stays reserved inside a layout
- [x] Measured keyframe size recorded before and after on the tictactoe config
- [x] VERIFY: go test -race ./... && (cd client && pnpm run typecheck && pnpm test)

## Work Log

### 2026-10-08T23:15:52.000Z - DECISION (Boss): the client sends the hash it holds, rather than the server tracking it per connection. Handlers never see the connection and REST holds none, so a client-held hash is stateless and identical on every transport. This replaces the story's "layout frame once per connection" wording: the layout travels in the keyframe it has always travelled in, and is left out when the client already has it.

### 2026-10-08T23:15:52.000Z - IMPLEMENTED. GameService.Keyframe(g, held) renders the sanitized game with layoutHash (SHA-256 of data.layout's JSON; map keys marshal sorted, so it is stable) and leaves data.layout out when held matches. GetJSONBytes(id, held), SeatCaller (join, create), refresh and reconnect all render through it, taking utils.HeldLayoutHash(payload). Lua indri.refresh / refreshSelf always send the layout whole (the script does not know what recipients hold). REST: layoutHash optional on reconnect, create, join, refresh. GraphQL: layoutHash: String on reconnect, createGame, joinGame, refresh (regenerated with gqlgen v0.17.95); TestSchema_RefreshTakesNoArguments became ...TakesNoArgumentNamingAGameOrACaller, allowing exactly layoutHash, which names content and never a game or caller. Client MessageHandler: holds {hash, layout} from a full keyframe, names it on the four keyframe actions, puts it back into a lean keyframe, drops it on any delta touching data / data.layout*, and asks for a plain refresh if a lean keyframe names a layout it does not hold. Deltas and sanitization are unchanged, so layout edits still reach every player as deltas and SanitizeDelta and Sanitize agree as before; privateData stays reserved in a layout (validators untouched).

### 2026-10-08T23:15:52.000Z - MEASURED. tictactoe keyframe: 4614 bytes with the layout, 725 without (-84%; story 083 projected 4936 -> 754 on the older model). Live server, a seated host: create 4653 bytes, refresh naming the hash 764 bytes with the same hash, a stale hash answered with the layout.

### 2026-10-08T23:15:52.000Z - VERIFY. New tests: game service 6 (carries layout and hash, omits held, stale/garbage hash sends it, hash follows an edit, no layout no hash, sanitized either way); handlers 4 (refresh, reconnect, join, create each leave out the held layout; each mutant ignoring the payload hash killed); REST 4 table cases; GraphQL 4-mutation pass-through; client 5 (all three cache mutants killed). go test -race ./... green with PostgreSQL; pnpm test 425/425; typecheck clean.

