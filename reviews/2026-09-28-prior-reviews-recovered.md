# Prior reviews of indri: what was recovered

Recovered 2026-09-28. The repo was not modified. `main` is at 3eab208.

## Bottom line

- **No full-codebase review from the last few weeks exists in any transcript we still have.** The
  reviews in sessions 58f12595 and e7deb4c6 (2026-09-27) were **branch-diff reviews**
  (`/code-review high main...wip/pluggable-transports`), not whole-codebase reviews.
- **A whole-codebase review almost certainly happened on 2026-09-09**, because the commit series
  6760eda..f684daf remediates P1/P2/P3 "review findings". Its findings file was
  `reviews/review-2026-09-09-fixes.md`, and `reviews/archive` is gitignored. That file is **no longer
  on disk**, and no transcript from before 2026-09-15 survives: clued's oldest indri session starts
  2026-09-21 and `~/.claude/projects` has nothing earlier either. So the Sep 9 findings can't be
  recovered word for word. What remains is the 4 archived stories that quote it, plus the commit
  messages.
- "Review finding #7" (slow clients: close instead of dropping) is item #7 of a **7-item forward list**.
  Session e7deb4c6 ("indri-3d") sent it to session 58f12595 ("pluggable-transport-system") at
  2026-09-27T21:12. That list was taken from the second branch-review run (Review B below).
- Session `aa8c14e1`, `0425e401`, `6420a509` and `b717cf88` contain plan/spec reviews and per-task
  spec-compliance/quality reviews from subagent-driven development only. None of them is a codebase
  review.

## Files on disk

| Path | Contents |
|---|---|
| `/Users/robbiebyrd/Projects/indri/stories/archive/001-96a1-…md` … `004-6ca2-…md` | Sep 9 findings SEC-1, BUG-1, BUG-2, DOC-1, quoted with `source_review: reviews/review-2026-09-09-fixes.md` |
| `/private/tmp/claude-501/-Users-robbiebyrd-Projects-indri/e7deb4c6-2ca6-48aa-acf9-2789ea15912d/scratchpad/dbfix/brief.md` | Boss-approved rulings for the DB-layer fixes from Review A |
| `/private/tmp/claude-501/-Users-robbiebyrd-Projects-indri/e7deb4c6-2ca6-48aa-acf9-2789ea15912d/scratchpad/dbfix/review.diff`, `rereview.diff` | Diffs used for re-review of the fixes |
| `/Users/robbiebyrd/.claude/projects/-Users-robbiebyrd-Projects-indri/e7deb4c6-2ca6-48aa-acf9-2789ea15912d.jsonl` | Transcript lines 997 (Review A JSON), 1012 (summary), 1105 (Review B JSON), 1120 (summary), 1149 (7-item forward list), 1454 (leftovers on 2026-09-28) |
| `/Users/robbiebyrd/.claude/projects/-Users-robbiebyrd-Projects-indri/58f12595-b0bd-4ce2-aaf2-7910e6ae4e33.jsonl` | Transcript lines 7986 (receives the 7 items), 8354/8359 (fix report), 8729/8747 (#5 decision, #7 folded into the plan) |
| `/Users/robbiebyrd/Projects/indri/plans/session-resume.md` (also in `/private/tmp/claude-501/wt-merge/plans/`) | "Slow clients: close instead of dropping (review finding #7)" section; bugs 2–5 status; hand-off notes |
| `/Users/robbiebyrd/Projects/indri/.remember/today-2026-09-27.md` | One-line log: "Code review: 10 issues — 4 bugs … 2 security … 2 race, 2 perf" |

The task output symlinks for the review agents (`…/e7deb4c6…/tasks/abfd2843523055115.output`,
`ae9975f492585792d.output`) no longer resolve. Their results survive only inside the transcript.

---

## Review 0: 2026-09-09 whole-codebase review (source file lost)

Reviewed `main` on 2026-09-09, before 6760eda. The session is not recoverable. Remediation commits
(all confirmed ancestors of `main`) and the findings they state they fix:

| Commit | Findings addressed (from the commit message) |
|---|---|
| 6760eda | create handler inverted type assertion panic / `private:true` ignored; infinite loop on game-code collision; phantom games from Get/GetByCode on repo error; `UnsetHost` set instead of cleared; melody MaxMessageSize wrong env var + `:=` shadow on singleton; Mongo URI logged, `log.Fatal`, `:=` shadow; malformed login-redirect JSON; debug `fmt.Println`; no `.dockerignore` |
| 56d1a6f | Enumerable ObjectID session tokens → 256-bit token; reconnect inverted GameID condition; kick authorized the target, not the caller; RemovePlayer called with a code; getConnectionForPlayer read the caller's key; no Origin allowlist (CSWSH); inquire panics on wrong types |
| 2997360 | Committed-red tictactoe test; swapped getBoardSize bindings; draw wedges the game; added CI |
| f0fb05c | x/text DoS (GO-2026-5970), mongo-driver CVE (GO-2026-5327) |
| a1e0be6 | No panic recovery in dispatch; websocket errors discarded; false disconnect error logs; bson tag mismatch froze createdAt/updatedAt; partial updates nulled fields; dead getDB copying a lock |
| 086c336 | No graceful shutdown; `log.Fatal` in Boot; DefaultServeMux, no timeouts; changestream shutdown deadlock |
| f39cc50, bdd1a8b | Client: inverted deleteBefore (data loss), pre-keyframe delta throw, keyframe mutation, prototype pollution, arrays made into maps, socket lifecycle, RN `<p>/<div>`, react-select breaks native |
| a9fa690, 88875e4 | Lost updates on concurrent game writes → version fence + distributed lock |
| 83c2be0, 7dc395f | Change-stream shortcomings; privateData leaked in deltas; sendToPlayer/Players/Team matched nothing |

### Second pass: `reviews/review-2026-09-09-fixes.md` (review of the fixes)

These four are recovered **verbatim** from `stories/archive/*.md`:

| ID | Sev | Title | Files | Status |
|---|---|---|---|---|
| SEC-1 (story 001-96a1) | P1 | Invalidate the session token on logout. Logout only called HandleDisconnect, and the bearer token stayed valid indefinitely (CWE-613) | `internal/handlers/actions/logout/handler.go`, `internal/repo/session/*`, `internal/services/session/session.go`, reconnect handler | **Fixed** f920bd6 (Delete + 7-day TTL index) |
| BUG-1 (story 002-c659) | P2 | Route the tictactoe move handler through Store.Mutate. Two non-atomic UpdateField writes lose updates or wedge mid-turn | `example/tictactoe/server/handlers/move/handler.go` | **Fixed** 9a1a338 |
| BUG-2 (story 003-916b) | P2 | Propagate the discarded Write error in register's already-logged-in branch | `internal/handlers/actions/register/handler.go:33` | **Fixed** 9a1a338 |
| DOC-1 (story 004-6ca2) | P3 | Fix stale "Storer out of date" claim in ARCHITECTURE.md | `docs/ARCHITECTURE.md:108` | **Fixed** 9a1a338 |

The remaining P3s are known **only from the f684daf commit message**. Their stories were never
committed, so the original finding text is lost:

| Story | Sev | Finding (commit text) | Status |
|---|---|---|---|
| #f624 | P3 | `UpdateGame.Private` omitempty prevented setting `private:false` | Fixed f684daf |
| #7d48 | P3 | Dockerfile didn't set `INDRI_LISTEN_ADDRESS=0.0.0.0` | Fixed f684daf |
| #d035 | P3 | login/reconnect on an already-authenticated connection stranded the prior session's presence | Fixed f684daf |
| #c546 | P3 | session.New duplicate-key race; game New surfaced raw E11000 | Fixed f684daf |
| #42ce | P3 | boot test checked presence only, not handler-package binding | Fixed f684daf |
| #d108 | P3 | global router registry not reset between tests | Fixed f684daf |

---

## Review A: branch review, run 1 (2026-09-27, session e7deb4c6)

`/code-review high main...wip/pluggable-transports`, reviewing snapshot **75e4cff**. Every finding was
re-checked against **9f50df6**. Line numbers are from 9f50df6. Findings are quoted verbatim from the
agent's JSON:

1. **internal/repo/game/memory.go:58.** "copyGame shallow-copies Stage (the Scenes map and the Stage data maps) and the Players pointer-to-map fields, and new games get script.Stage by assignment, so a Mutate apply that writes g.Stage.Scenes[...] changes the stored game, every other game made from the same script, and the script template itself." Failure: "The Lua handler (luahandler/runtime.go: `g.Stage.Scenes[currentScene] = scene`) runs in game A: the scene data shows up in game B and in Script.Stage, so a reset no longer restores defaults. Two games running Lua handlers at once … 'fatal error: concurrent map writes'. Also, if apply returns an error or ErrAbort, the write has already landed in stored state and is never rolled back."
   **Status: FIXED** 0bf1a50 (deep copy), with follow-ups 394aa8a and abb930d.
2. **internal/repo/game/sqlite.go:153.** "Exists on the SQLite, Postgres and memory game stores looks up by game id (memory.go:172, postgres.go:171), but GameService.New and getAutoGeneratedGameCode pass a game code, which is what MongoStore.Exists matches on." Failure: "Exists(code) always returns false. The loop that retries on an autogenerated-code collision never retries … duplicate-key error on create …"
   **Status: FIXED** 1fd7d2c.
3. **internal/repo/session/sqlite.go:127.** "The SQLite and memory session stores (session/memory.go:88) never enforce sessionMaxAge, unlike the Mongo TTL index and the Postgres created_at cutoff, so session bearer tokens never expire." Failure: "a token that leaked … months ago still works in `reconnect` via GetByToken."
   **Status: FIXED** 6b8bdd4, with migration hardening in 8c7d56f. Follow-up: Boss has since decided on 24 h *idle* expiry on every store, plus an on-read check so Mongo stops relying on the asynchronous TTL sweep. That work is handed to the session-resume build and is **open**.
4. **internal/repo/game/mongo.go:81.** "Model IDs changed from bson.ObjectID to string and the Mongo stores now filter `_id` by a raw string (user/mongo.go:103, session/mongo.go:108), with no migration, so documents already stored with ObjectID `_id`s can no longer be found by id." Failure: users registered before the upgrade can't log in or reconnect, and older games disappear.
   **Status: ACCEPTED / DEFERRED.** The spec made it a deliberate clean break (spec line 44). It still needs a release note, which is **open**.
5. **internal/repo/session/sqlite.go:192.** "Session Update on SQLite (and Postgres, postgres.go:242) reads the whole JSON blob, patches it in Go and writes all of it back with no lock or version check, which replaces Mongo's atomic per-field $set. The SQLite version also ignores RowsAffected (the ErrNotFound fix only went to Postgres) and never updates the user_id column." Failure: concurrent join and team change lose GameID or TeamID.
   **Status: PARTLY FIXED.** 68ba7b6 covers RowsAffected/ErrNotFound and the user_id column, and 33bf874 covers ErrDuplicate. The **lost-update race from rewriting the whole blob was not addressed** and is **open**; I found no commit that fixes it.
6. **internal/repo/game/memory.go:304.** "The memory store publishes the change event while holding the store-wide s.mu write lock, and the Redis publish or the InProcess channel send (256 buffer, blocks when full) can block."
   **Status: FIXED** e2fe3b3.
7. **internal/transport/webrtc/webrtc.go:302.** "OnMessage is registered alongside OnOpen, but pion runs the OnOpen handler on its own goroutine while the read loop starts separately, so a message can be delivered before Hub.Add and Connected run." Failure: a `login` sent in the client's onopen is handled before the conn is in the hub, so broadcasts miss it.
   **Status: OPEN.** At 3eab208, `webrtc.go:289-304` still registers OnOpen (Hub.Add, Connected) and OnMessage (Deliver) independently.
8. **internal/repo/game/sqlite.go:29.** "The Storer compile-time assertion for SQLiteStore is commented out behind a temporal TODO."
   **Status: FIXED** 3968d2a.
9. **internal/repo/session/sqlite.go:157.** "SQLite session Find (and user Find, user/sqlite.go:130) reads and JSON-decodes every row on every call, ignoring the indexed token, user_id and email columns, and the pool is pinned to one connection."
   **Status: OPEN.** At 3eab208, `session/sqlite.go:158` still runs `SELECT data FROM sessions WHERE created_at > ?` and filters in Go.
10. **internal/transport/origin.go:38.** "The origin check now also allows any Origin whose host equals the request's Host header … this new rule lets DNS-rebinding pages through (the scheme is ignored as well)."
    **Status: OPEN, needs a decision.** At 3eab208 it is unchanged: `strings.EqualFold(u.Host, r.Host)`. It was flagged as the gorilla default and a judgment call.

The agent's "didn't make the cap" items:
- `repos.go:61`: the SQL pool is never closed if a later store constructor fails. **OPEN.** `internal/injector/repos.go` still returns without closing `sqlDB`.
- `internal/repo/errors.go:11`: `repo.ErrConflict` is unused and duplicates `mutation.ErrConflict`. **OPEN.** Still defined, with no references.
- `isPgDuplicateKey` / `isSQLiteConstraintUnique` are copied across packages and match on error text. **OPEN.** `isPgDuplicateKey` is still in `user/`, `game/` and `session/postgres.go`.
- Already fixed at 9f50df6 and dropped by the reviewer: `_ =` on Publish errors, and game-store logic copied three times.

## Review B: branch review, run 2 (2026-09-27, session e7deb4c6)

The same command was re-run on **897f140** (after the merge). Quoted verbatim:

1. **internal/repo/session/sqlite.go:200.** "Session Update ignores UpdateSession.SlotID in the SQLite, Postgres (postgres.go:249) and Memory (memory.go:141) stores; only the Mongo store saves it." Leave, kick and disconnect are broken on non-Mongo backends.
   **FIXED** 191813c (the transports session's fix, via shared `fields.go`).
2. **internal/handlers/actions/layout/handler.go:57.** "The layout host check still looks players up by user ID … but players are now keyed by slot ID."
   **FIXED** 0f85d3b (shared `handlerUtils.RequireHost`).
3. **client/services/message-handler.ts:193.** "setSchema(g) keeps a reference to the keyframe object, not a copy, so the layout merge … inserts "layout" into the schema that resolvePath sorts." A `turn` delta lands on `layout`.
   **FIXED** 7838e56.
4. **internal/repo/game/operations.go:124.** "assignSlot never checks whether userId already holds a slot." Ghost slots and a ghost host.
   **FIXED** ca0ec32.
5. **internal/repo/game/operations.go:152.** "removePlayer and setConnected act on a slot ID without checking that slot.UserID belongs to the caller. Meanwhile leave and kick never clear the session's GameID/SlotID …"
   **PARTLY FIXED** ca0ec32: operations now check the holder. **Clearing the session's GameID, SlotID and TeamID on leave/kick is OPEN** and handed to the session-resume build (`plans/session-resume.md` bug 2).
6. **internal/repo/game/memory.go:54.** Shallow `copyGame`; Mutate edits stored state in place. The same issue as Review A #1.
   **FIXED** 0bf1a50.
7. **internal/repo/game/memory.go:304.** Publish under `s.mu` gives a confirmed deadlock with the monitor's OpKeyframe → Get → RLock.
   **FIXED** e2fe3b3.
8. **internal/services/events/clientview.go:12.** "ClientView strips data.layout from every keyframe and delta, and the LayoutFrame always carries i.LayoutData, which is computed once at boot … Runtime edits from the `layout` action therefore never reach any client."
   **FIXED.** Boss chose option (a), per-game layout: e465933, 4863a54, 63abab7.
9. **internal/services/events/delta.go:76.** "When an array shrinks, diffSlice emits per-index removals. The client applies them with `delete arr[i]`, which leaves holes …"
   **FIXED** 7838e56 (the client truncates).
10. **internal/transport/hub.go:106.** "QueuedConn.enqueue silently drops a frame when a slow client's buffer is full. The broadcaster only logs it and the connection stays open …" Fix: close on ErrBufferFull.
    **OPEN / DEFERRED** by design, to be built together with session resume. It is written up in `plans/session-resume.md` §"Slow clients: close instead of dropping (review finding #7)". At 3eab208, `hub.go:98-108` still returns `ErrBufferFull` without closing.

Review B's cleanup items were left out because of the 10-finding cap, and all are **OPEN**:
- The `*_host.go` / `*_player.go` wrappers and the snapshot-and-diff logic are duplicated across all four game stores.
- `authResponse` is defined twice, in login and reconnect.
- Dead pass-through branch in `SanitizeDelta`.
- One MessagePack encode per recipient in `broadcastToSessions`.
- SQL session `Find("gameId")` scans the full table.

### The "7-item" numbering used in session 58f12595

e7deb4c6 forwarded the transport-side subset to 58f12595 and renumbered it:

| # | Maps to | Status |
|---|---|---|
| #1 | B2 layout host | Fixed 0f85d3b |
| #2 | B4 assignSlot | Fixed ca0ec32 |
| #3 | B5 stale session | Partly fixed ca0ec32; session clearing open |
| #4 | B3 setSchema | Fixed 7838e56 |
| #5 | B8 layout never reaches clients | Fixed e465933 (option a) |
| #6 | B9 array holes | Fixed 7838e56 |
| #7 | B10 slow clients | Open; deferred into the session-resume plan |

All fix commits were merged via `wip/review-fixes` (cc400e2 → 48a9bda). Every hash above is confirmed to
be an ancestor of `main` (3eab208).

## New items raised in the 2026-09-28 leftovers list (e7deb4c6 line 1454)

- SQLite has no `busy_timeout`, so a second server opening the same file fails with "database is locked". **OPEN.** There is no `busy_timeout` in `internal/clients/sqlite/client.go`.
- The Postgres DDL has indexes that duplicate what its UNIQUE constraints already create. **OPEN** (not verified).
- Postgres rejects strings containing a NUL character (`\u0000`) with an unclear error. **OPEN** (not verified).
- CI's new Postgres job has never been confirmed green. **Unknown.**
