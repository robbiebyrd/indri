---
date: "2026-09-09"
author: "Robbie Byrd"
reviewers: "Multi-Agent (security, architecture, simplicity, silent-failure, test-quality, go, data-safety)"
pr: ""
branch: "main"
target_ref: "e058d20"
confidence_threshold: "70"
validated: "true"
status: "triaged"
triaged_at: "2026-09-09T18:58:52Z"
triaged_by: "Robbie Byrd"
findings_total: "10"
findings_promoted: "10"
findings_skipped: "0"
stories_created: "001-96a1:SEC-1, 002-c659:BUG-1, 003-916b:BUG-2, 004-6ca2:DOC-1, 005-f624:BUG-3, 006-7d48:OPS-1, 007-d035:SEC-2, 008-c546:DATA-1, 009-42ce:TEST-1, 010-d108:TEST-2"
---
# Code Review: commit e058d20 "Fixes" (+ whole-codebase pass)

**Date:** 2026-09-09
**Target:** commit e058d20 on `main` (logout wiring, Storer interface assertions, register-handler key fix, registration-coverage test, docs, Dockerfile). Reviewers were also allowed to range across the codebase.

## Summary
- **P1 Critical:** 1
- **P2 Important:** 2
- **P3 Nice-to-Have:** 1 (above threshold) + 6 below threshold
- **Confidence Threshold:** 70
- **Filtered Out (below threshold):** 6
- **Validated:** logout P1 flow-traced & confirmed; register fix and handlers_test independently confirmed sound

The commit itself is largely sound: the `register` key fix (`sessionId` vs the never-set `userId`) correctly revives dead guard code; the `var _ Storer = (*Store)(nil)` assertions genuinely restore drift protection and now match `*Store`; the registration-coverage test is real, non-flaky, and needs no DB. The one serious issue is a consequence of *wiring `logout`*: it exposes a logout that doesn't actually end the session.

---

## P1 - Critical (Block Merge)

### [SEC-1] `logout` does not invalidate the session token
**File:** `internal/handlers/actions/logout/handler.go:22`
**Reviewer:** security-reviewer
**Confidence:** 90
**Severity:** P1
**Validated:** Confirmed — grep shows zero delete/invalidate/revoke/TTL anywhere in the session repo/service; `logout.Handle` only calls `HandleDisconnect` (closes the socket, marks the player disconnected). The 256-bit bearer `Token` on the `Session` document is never removed or rotated.
**Issue:** This commit wires the `logout` action for the first time, so it is now client-reachable. But logout only closes the WebSocket — the session token stays valid indefinitely (no revoke, no expiry, and `sessionRepo.New` even reuses the same token across logins). Anyone still holding that token can `{"action":"reconnect","sessionId":"<token>"}` and resume full authenticated access after the user believes they logged out (CWE-613). Keyframe/UI imply the session ended; it did not.
**Fix:** Add `Delete`/`Invalidate` to `sessionRepo.Store` + `Storer` + the session service, and call it from `logout.Handle` (delete or rotate the token) before closing the connection. Add a TTL/idle-expiry on the `session` collection as defense in depth.

---

## P2 - Important (Fix Before/After Merge)

### [BUG-1] tictactoe move handler bypasses `Mutate`, doing two non-atomic writes
**File:** `example/tictactoe/server/handlers/move/handler.go:130,146`
**Reviewer:** data-safety-reviewer
**Confidence:** 85
**Severity:** P2
**Issue:** The move handler does two independent `GameRepo.UpdateField` calls ("stage" then "teams") built from a stale in-memory read, instead of the `Store.Mutate` CAS primitive the repo now provides for exactly this. Two racing moves can lose an update (last `$set` wins, no version check), and a crash between the two writes leaves the board advanced but the turn flag unflipped — an inconsistent, unrecoverable mid-turn state. `ARCHITECTURE.md` itself states every multi-field game edit must go through `Mutate`; this is the one place that doesn't. (Pre-existing; in scope for the whole-codebase pass.)
**Fix:** Rewrite the handler body as a single `GameRepo.Mutate(gameID, func(g *models.Game) error { …update stage + teams in place… })`, matching `internal/repo/game/team.go`.

### [BUG-2] register's "already logged in" branch discards the write error
**File:** `internal/handlers/actions/register/handler.go:33`
**Reviewer:** silent-failure-hunter
**Confidence:** 85
**Severity:** P2
**Issue:** `_ = ss.Write(authExistsErrorMessage)` discards the write error and `Handle` still returns `nil`. If that write fails, there is no log and the caller sees success, while the client never got the "already logged in" message — a debugging dead end. The success path in the same file correctly checks its write error, so this branch is inconsistent.
**Fix:** `if err := ss.Write(authExistsErrorMessage); err != nil { return fmt.Errorf("notify already-registered session: %w", err) }`.

---

## P3 - Nice-to-Have (above threshold)

### [DOC-1] `ARCHITECTURE.md` still calls the game `Storer` "out of date"
**File:** `docs/ARCHITECTURE.md:108`
**Reviewer:** architecture-reviewer, data-safety-reviewer (deduped)
**Confidence:** 95
**Severity:** P3
**Issue:** The doc says the `Storer` interfaces are "unused and, for `game`, out of date." This commit restored full parity and added the compile-time assertion, so "out of date" is now false and contradicts the code it describes. ("Unused as an abstraction" is still true — no consumer takes a `Storer`.)
**Fix:** Drop "and, for `game`, out of date"; state the assertion is a drift guard and note nothing consumes the interface as an abstraction yet.

---

## Below Threshold (<70) — noted, not blocking

- **[BUG-3]** `UpdateGame.Private bool` has `bson:"private,omitempty"` (`internal/models/game.go:52`) — a future `Update` that sets `Private:false` to un-privatize a game silently drops the field. Latent (no caller today). Conf 55. Fix: remove `omitempty` (or use `*bool`).
- **[SEC-2]** `login`/`reconnect` lack the re-auth guard `register` now has — an authenticated connection can rebind to another user without logout, leaving user A showing `connected:true` forever (stale presence, not privilege escalation). Conf 55.
- **[DATA-1]** `session.New` / `game.Service.New` are check-then-act TOCTOU; unique indexes prevent duplicates but the raw Mongo `E11000` surfaces instead of the friendly "already exists" behavior the docs promise. Pre-existing. Conf 60–70. Fix: catch dup-key and re-fetch/translate.
- **[TEST-1]** `handlers_test.go` verifies action-name *presence* but not correct handler *wiring* — swapping two entries would still pass. Conf 65. Fix: also assert `Name`/package matches the action.
- **[TEST-2]** `router.registeredHandlerMap` is a mutable global with no reset; harmless today (single caller) but a latent test-isolation trap. Conf 55.
- **[OPS-1]** Dockerfile `EXPOSE 5002` is correct, but `INDRI_LISTEN_ADDRESS` defaults to `localhost`, so the containerized server binds loopback and isn't reachable via a published port without `INDRI_LISTEN_ADDRESS=0.0.0.0`. Conf 40. Fix: `ENV INDRI_LISTEN_ADDRESS=0.0.0.0` in the image.

## Dismissed / Accepted

- **Unconsumed `Storer` interfaces** (simplicity, go, architecture; conf 45–90): flagged as dead abstraction, but `CLAUDE.md:223` documents them as an intentional compile-time drift-guard convention. Accepted, not a defect. Don't describe them as a decoupling seam.

---

## Cross-Cutting Analysis

**Root cause — session lifecycle is incomplete (SEC-1, SEC-2, DATA-1):** the session model has no invalidation, no expiry, and racy create-or-reuse. Wiring `logout` turned the missing-invalidation half into a live security gap. A single "session lifecycle" work item — add `Invalidate`/`Delete` + TTL + idempotent create — closes SEC-1, SEC-2's cleanup path, and DATA-1's session case together.

**Single-fix opportunity — route the move handler through `Mutate` (BUG-1)** removes the last non-atomic multi-field game write in the tree, making "all game edits go through Mutate" universally true and matching the architecture doc.

## Recommended Actions
1. **Immediate (before relying on logout):** SEC-1 — make logout actually invalidate the token.
2. **This change:** BUG-2 (one-line), DOC-1 (doc), BUG-1 (move handler → Mutate).
3. **Follow-up:** the session-lifecycle work item (SEC-2, DATA-1, TTL) and the sub-threshold hygiene items.
