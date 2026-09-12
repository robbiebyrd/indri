---
date: "2026-09-12"
author: "Robbie Byrd"
reviews: "plans/layout-authoring-editor.md"
reviewers: "Multi-Agent (security, performance, architecture, simplicity, silent-failure, test-quality, go, typescript, react, data-safety)"
branch: "main"
target_ref: "941d09d"
confidence_threshold: "70"
validated: "true"
status: "open"
---
# Code Review: layout op decode/validate (untracked) + client Lua bridge (941d09d)

**Target:** untracked `internal/handlers/actions/layout/` + `internal/transport/graphql/graphql_test.go`, plus the client/config changes committed in `941d09d`. ~3,250 lines, 15 files.

Validation note: instead of spawning finding-validator agents, the highest-severity findings were verified directly against source in the parent session (`kick/handler.go:66`, `delta.go:78`, `validate.go:604`, `widget-host.tsx:87`, `graphql.go:105`, `handlers_test.go`). Those are marked **Verified**.

## Summary

- **P1 Critical:** 2
- **P2 Important:** 11
- **P3 Nice-to-Have:** 12
- **Below threshold (70), not listed:** 8
- **Agent coverage:** 10/10 reported. None could write to the review dir (Read/Grep/Glob toolsets), so all reports were collected inline.

Headline: the new Go layout package is the strongest code in the diff — fail-closed, comma-ok assertions throughout, every cap tested at its boundary, and no P1/P2 from either the Go or test-quality reviewer. Both P1s are **pre-existing** bugs on the paths this work depends on, not defects in the new code.

---

## P1 - Critical

### [BUG-1] `kick` force-disconnects a player it failed to remove
**File:** `internal/handlers/actions/kick/handler.go:66-72`
**Reviewer:** silent-failure-hunter · **Confidence:** 95 (**Verified**) · **Severity:** P1
**Issue:** `RemovePlayer`'s error is logged, `err` is discarded, `Handle` returns `nil`, and the target is force-disconnected regardless. The player is cut off from the transport while remaining a player of record in Mongo — they cannot rejoin cleanly and cannot play. The comment directly above the return ("an offline target has still been removed above") is false on that path.
**Fix:** return the wrapped error and skip the disconnect: `return actions.Result{}, fmt.Errorf("removing player %v from game %v: %w", targetUserId, gameId, err)`.

### [BUG-2] Handler errors are invisible to WebSocket clients
**File:** `internal/services/boot/handlers.go:130-132`
**Reviewer:** silent-failure-hunter · **Confidence:** 95 (**Verified**) · **Severity:** P1
**Issue:** `handleClientMessage` only `log.Printf`s `dispatchErr`; it never converts it to a `models.WSError` in `Responses`. GraphQL (`resolvers/resolver.go:82`) and REST (`rest/rest.go:115`) both propagate the error. A WS client that sends an invalid action sees nothing at all. CLAUDE.md documents the convention ("handlers return `err.BytesError()` in `actions.Result.Responses`") but `kick` and the other built-ins do not follow it, so the convention is currently unimplemented anywhere.
**Fix:** decide one of the two paths and document it **before** `layout/handler.go` (story 027) is written — otherwise every carefully-worded error `validate.go` produces will never reach the host who triggered it, defeating the point of it being the security boundary.

---

## P2 - Important

### [SEC-1] Unconstrained scene/widget ids collide with the dot-path redaction mechanism
**File:** `internal/handlers/actions/layout/validate.go:240`, `op.go:251`
**Reviewer:** security · **Confidence:** 85 (**Verified**, raised from 70) · **Severity:** P2
**Issue:** Ids are validated for non-emptiness only. `events.joinPath` (`delta.go:121`) builds delta paths by raw `prefix + "." + key`, and `pathHasSegment` (`delta.go:78`) splits on `.` and compares segments to `"privateData"`. A widget id `"foo.privateData"` yields a path whose trailing segment is literally `privateData`, so `SanitizeDelta` silently drops every update to that widget from the broadcast.
Crucially, `findReservedKey` (`validate.go:604`) uses **exact key equality** (`key == reservedKey`), so `"foo.privateData"` is a single key that is *not* equal to `"privateData"` and passes the reserved-key scan. The two mechanisms disagree precisely on dotted ids.
**Fix:** the systemic fix is at the `events` layer — escape segments or carry paths as `[]string` — because a charset allowlist must then be re-added at every future producer. The tactical fix is `^[A-Za-z0-9_-]+$` on ids in `validate.go`, mirrored in the TS schema.
**Note:** the same root cause was independently found by the second opinion against `plans/lua-game-scripting.md` (arbitrary Lua table keys). One defect, two entry points.

### [PERF-7] GraphQL has no request body size limit; REST and WS both do
**File:** `internal/transport/graphql/graphql.go:105`
**Reviewer:** performance · **Confidence:** 90 (**Verified**) · **Severity:** P2
**Issue:** `rest.go:127` wraps bodies in `http.MaxBytesReader` at 64 KB and `ws.go:50` sets `MaxMessageSize`; `mux.Handle(path, authMiddleware(t.srv))` wraps nothing, and `entrypoints/http/server.go` sets only `ReadTimeout`. Layout ops (`setScript.source`, opaque `config`/`style`) are exactly the large-payload fields this exposes.
**Fix:** wrap the GraphQL handler in `http.MaxBytesHandler` at a limit consistent with REST.

### [BUG-3] Every widget remounts once after board mount
**File:** `client/components/board/widget-host.tsx:87`
**Reviewer:** react · **Confidence:** 85 (**Verified**) · **Severity:** P2
**Issue:** `PressTarget` returns `<>{children}</>` when `onPress` is undefined and `<Pressable>` otherwise. `useLuaBridge` creates the bridge in an effect, so `onPress` is undefined on first render and defined after — the element type at that position flips Fragment→Pressable and React unmounts/remounts every widget subtree. Harmless for today's stateless text widget; a landmine for any stateful one.
**Fix:** always render `Pressable` for non-subgrid widgets with `onPress={() => onPress?.(id)}`, branching only on `type === SUBGRID_TYPE`, which never changes for a mounted widget.

### [PERF-1] Full game-state clone + deep-freeze on every delta
**File:** `client/layout/lua/host-api.ts:323-329`
**Reviewer:** performance · **Confidence:** 90 · **Severity:** P2
**Issue:** `setSnapshot` runs `JSON.parse(JSON.stringify(game))` plus a recursive `deepFreeze` over the whole game on every delta, with no diffing — O(total state) per tick even for changes no script observes. The `stateRef` cache is dropped unconditionally on each call, so it amortizes nothing across deltas.
**Fix:** give the bridge exclusive ownership of the object `GameStateParser` already builds, or make the Lua-side marshal incremental.

### [PERF-2] `GameStateParser.reapply()` replays all deltas on every delta
**File:** `client/services/game-state-parser.ts:47-73`
**Reviewer:** performance · **Confidence:** 75 · **Severity:** P2 (pre-existing)
**Issue:** Deep-clones the keyframe and replays every delta since it, per incoming delta — quadratic between keyframes. Combined with PERF-1, each tick pays two full-tree JSON round-trips regardless of delta size. Also the root cause of the already-acknowledged `BoardView` re-parse (`board-view.tsx:55`).
**Fix:** apply deltas incrementally onto `currentState`. Separate story; not this diff.

### [TYPE-1] `state()` is typed shallow-readonly but documented and frozen deep
**File:** `client/layout/lua/host-api.ts:74`
**Reviewer:** typescript · **Confidence:** 65 · **Severity:** P2
**Issue:** `Readonly<Game>` freezes only top-level properties, so `state().stage.currentScene = "x"` type-checks despite the doc comment promising a deep-frozen clone and the runtime enforcing it.
**Fix:** `ReadonlyDeep<Game>` from `type-fest`, already a devDependency. Type-only usage, so it stays in devDependencies.

### [DATA-1] `addWidget` can silently overwrite an existing widget
**File:** `internal/handlers/actions/layout/op.go:94`
**Reviewer:** data-safety · **Confidence:** 70 · **Severity:** P2 (blocks story 027)
**Issue:** `validateLayout` inspects the *resulting* document and structurally cannot distinguish "created" from "overwrote". The only stated protection is a client-side rule, and this repo's own ADR says the client is not a security boundary. Same gap in reverse for `setPlacement`/`setWidgetConfig`/`setStyle`/`setScript` targeting a non-existent scene or widget.
**Fix:** story 027's `applyLayoutOp` must check existence itself. Add it as an explicit acceptance criterion — `validateLayout` will never cover it.

### [DATA-2] First `setGrid` on a layout without `scenes` cannot succeed
**File:** `internal/handlers/actions/layout/validate.go:163-166`
**Reviewer:** data-safety · **Confidence:** 60 · **Severity:** P2
**Issue:** `scenes` is unconditionally required, but the whole layout is re-validated after each op, so a `setGrid` on a fresh layout fails. Already noted in story 026's worklog and handed to 027 with no pinning test.
**Fix:** seed `scenes: {}` in `applyLayoutOp`, or make `scenes` optional. Pick one and test it.

### [BUG-4] StrictMode would leave a permanently dead WebSocket
**File:** `client/app/index.tsx:25-46`
**Reviewer:** react · **Confidence:** 65 · **Severity:** P2 (dormant)
**Issue:** `ws` is created lazily during render but torn down in an effect cleanup. Under StrictMode's setup→cleanup→setup cycle, render is not re-run between the two setups, so the second setup reuses the closed `MessageHandler`. No `<StrictMode>` wrapper exists today, so it does not fire — but it will the moment one is added. `useLuaBridge` already documents and avoids exactly this.
**Fix:** create and dispose the `MessageHandler` inside an effect, mirroring `useLuaBridge`.

### [BUG-5] `EmitOutcome.reentrant` is reported nowhere
**File:** `client/layout/lua/bridge.ts:138,141`, `client/app/index.tsx:63`
**Reviewer:** silent-failure-hunter · **Confidence:** 60 · **Severity:** P2
**Issue:** `events.ts:14-19` documents that re-entrancy refusal "is reported in the result rather than thrown, so the caller can log it" — and every call site discards the `EmitOutcome`. Script errors are separately surfaced via `runtime.ts`'s `onError`, but a refused dispatch produces no signal anywhere: an entire `stateChanged`/`sceneChanged`/`widgetPress` broadcast is skipped silently. No test exercises the reentrant path.
**Fix:** log `reentrant` and non-empty `errors` at the call sites, or route them through the existing `onError` channel.

### [BUG-6] Socket errors and closes are `console.log`-only, now starving the Lua runtime
**File:** `client/services/message-handler.ts:84-92`
**Reviewer:** silent-failure-hunter · **Confidence:** 65 · **Severity:** P2 (pre-existing, amplified)
**Issue:** `onerror`/`onclose` carry acknowledged TODOs and log a bare event. This file is part of this changeset (it gained the `observe()` seam `LuaBridge` depends on), so a dropped connection now also silently stops every board script from receiving state, with no surfaced signal. Separately, `routeIncomingMessage:97-111` drops unrecognized messages with no logging at all.
**Fix:** not asking for reconnect logic — just name the failure (`console.warn("websocket closed", e.code, e.reason)`) so it is distinguishable from "nothing happened".

---

## P3 - Nice-to-Have

- **[SEC-2]** `findReservedKey` (`validate.go:604`) recurses with no depth cap, unlike the depth-4-capped subgrid walk. 256 KB of `{"a":{"a":...}}` reaches tens of thousands of levels. Found independently by security **and** performance; **verified** no depth param. Confidence 85.
- **[SEC-3]** `gameUpdates(gameId)` (`schema.resolvers.go:88`) never reads `gameID` — scoping is server-side via the session. **Verified**: no leak, but a decorative argument that invites a future maintainer to trust it. Drop it or enforce `gameId == session.GameID`. Confidence 90.
- **[TEST-1]** `readInt`'s NaN/Inf guard (`validate.go:549`) is unreachable via `validateLayout` because `checkSize:193` marshals first and `encoding/json` rejects NaN/Inf. The test at `validate_test.go:699` asserts only `err != nil`, so it passes while exercising `checkSize`. **Verified, with a correction to the original finding:** the same guard's `f != math.Trunc(f)` clause *is* reachable, so the branch is not dead — two of three conditions are. Confidence 85.
- **[TEST-2]** No accept-case at exactly `maxBytes`, so a `>` vs `>=` off-by-one at `validate.go:198` is uncaught.
- **[TEST-3]** `checkKeys` has pinned tests at layout/widget/placement level but not for a scene object or a sub-grid `config`.
- **[TEST-4]** Nothing binds the Go and TS copies of `minDim`/`maxDim`/`maxDepth`, despite `validate.go:13-19` stating they must stay identical. Both could drift to the same wrong value.
- **[PERF-3]** `checkSize` marshals the whole document just to measure it, once per op inside the lock — and `mutation.Run` retries up to 10×.
- **[PERF-4]** `validateLayout` re-validates the entire tree per op, including O(k²) overlap checks. Safe at `maxWidgets = 300`; a scaling risk if that cap rises.
- **[PERF-5]** No `maxScenes` cap — scene count is bounded only incidentally by `maxBytes`.
- **[SIMPL-1]** The `json.Number` case in `number()` (`validate.go:578`) is unreachable: neither `utils.DecodeMessageWithAction` nor the GraphQL JSON scalar calls `Decoder.UseNumber()`.
- **[STYLE-1]** `validate.go:388-452` uses bare `"col"`/`"row"`/`"w"`/`"h"`/`"left"`/`"top"` literals while the rest of the file hoists every wire field into a constant specifically to stop allowlist/reader drift.
- **[STYLE-2]** `validate.go` at 641 lines splits naturally into structural walk / placement / primitives.

---

## Cross-Cutting Analysis

### Root causes

| Root cause | Findings | Single fix |
|---|---|---|
| `events` builds delta paths as unescaped dotted strings | SEC-1, plus the Lua-plan key finding | Escape segments or use `[]string` paths in `events.Diff`/`SanitizeDelta` — fixes both entry points at once |
| No structural sharing across deltas on the client | PERF-1, PERF-2, PERF-8 | Make `GameStateParser` incremental; the bridge and `BoardView` costs both collapse |
| Errors are logged rather than returned to the caller | BUG-1, BUG-2, BUG-5, BUG-6 | Decide the WS error-response convention and apply it in `handleClientMessage` |
| Caps applied to the schema-shaped tree but not the opaque subtrees | SEC-2, PERF-5 | One shared depth/count guard covering `config`/`style` |
| Transports disagree on inbound limits | PERF-7 | `http.MaxBytesHandler` on GraphQL |

### Informational — not a defect

`internal/handlers/actions/layout/` has no `handler.go` and is unregistered, so `TestRegisterHandlers_CoversEveryActionPackage` fails and `go test ./...` is red on `main` (confirmed by running it). Story 025's worklog documents this as intentional until 027 lands, and another session owns it. Flagged by architecture, go and security reviewers; recorded here only so it is not re-reported.

### Context files to read before fixing

| File | Why | Referenced by |
|---|---|---|
| `internal/services/events/delta.go` | `joinPath`/`pathHasSegment`/`SanitizeDelta` — the root of SEC-1 | security, data-safety, performance |
| `internal/services/boot/handlers.go` | The dispatch-error swallow every handler inherits | silent-failure, architecture |
| `client/services/game-state-parser.ts` | Upstream of every client perf finding | performance, react |
| `plans/layout-authoring-editor.md` | Steps 3/4 are where DATA-1 and DATA-2 must be closed | data-safety, architecture, simplicity |
| `client/layout/lua/runtime.ts` | The `guard`/`onError` spine that makes most Lua failures non-silent | silent-failure, security |

## Recommended Actions

1. **Before story 027's `handler.go`:** settle BUG-2 (how a handler error reaches a WS client), and add DATA-1/DATA-2 as acceptance criteria. Both structurally cannot be fixed later by `validateLayout`.
2. **This change:** BUG-1 (`kick`), PERF-7 (GraphQL body cap), BUG-3 (`PressTarget`), SEC-1 id charset.
3. **Follow-up stories:** PERF-1/PERF-2 (client delta cost), SEC-1 proper fix at the `events` layer, the P3 test gaps.
