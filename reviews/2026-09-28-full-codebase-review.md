# Full codebase review — 2026-09-28

Reviewed: `main` at 3eab208 (after PR #2, `wip/pluggable-transports`, merged). Branch: `wip/codebase-review`.
Scope: all of `cmd/`, `internal/`, `example/`, `client/` (~21k lines of server Go, ~9k lines of client TS),
plus Docker/Compose/CI and the docs.

## How this review relates to earlier ones

The last whole-codebase review was on **2026-09-09**. Its findings file (`reviews/review-2026-09-09-fixes.md`)
and its transcript are gone; what survives, and every finding from the two 2026-09-27 branch reviews, is
recovered with its current status in [`2026-09-28-prior-reviews-recovered.md`](2026-09-28-prior-reviews-recovered.md).
This review supersedes it: every prior finding still open is carried forward below with a new ID, and every
prior "fixed" finding was re-checked against 3eab208. **No prior fix has regressed.** Two prior fixes left gaps
(kick authorization → F-01; privateData stripping → F-09).

## Method

1. Six reviewers each took a slice and re-checked every prior finding in it before looking for new ones.
   Their full reports, with file:line evidence, failure scenarios and suggested fixes for **every** finding
   (P1–P4), are the appendices in [`2026-09-28/`](2026-09-28/):
   `transport.md` (TR-*), `gamestore.md` (GS-*), `session-boot.md` (SB-*), `handlers.md` (HS-*),
   `deltas.md` (DP-*), `client-layout-docs.md` (CL-*).
2. Two independent verifiers then tried to **refute** every P1 and P2 by reading callers and guards, and
   reproducing where cheap ([`verify-server.md`](2026-09-28/verify-server.md),
   [`verify-client.md`](2026-09-28/verify-client.md)). **None was refuted.** Nine were downgraded; the
   severities below are the post-verification ones.
3. Duplicates across slices were merged into one `F-` ID.

Severity: **P1** security hole / data loss / crash reachable by a client · **P2** real bug in normal use ·
**P3** edge-case bug or notable design debt · **P4** cleanup.

Checks run at 3eab208: `go vet ./...` clean; `go test -race` green on every package that has tests
(memory, SQLite and Mongo stores exercised; **Postgres tests skipped** — no `INDRI_TEST_POSTGRES_URI`).
Client (in the main checkout, identical client code): `pnpm test` 249 pass, `typecheck` clean, `lint`
0 errors / 7 warnings. **Test coverage is the weakest area:** ~20 handler/service packages, `lock`,
`repo/utils`, `repo/script`, the Mongo/Redis clients and `cmd/server` have no tests, and CI has no Mongo
or Redis service so those tests always skip there (SB-17).

## Totals

| | P1 | P2 | P3 | P4 | Total |
|---|---|---|---|---|---|
| As reported by the six slices | 4 | 23 | 56 | 35 | 118 |
| After verification and merging duplicates | 3 | 12 | 59 | 33 | 107 |

Verification moved CL-2 from P1 to P2 and nine P2s to P3 (DP-1, DP-3, SB-4, GS-1, CL-3, CL-4, TR-1, TR-2,
TR-3). Merged duplicates: SB-1+HS-3, HS-9+DP-4, HS-6+TR-13+HS-18, DP-1+GS-9, DP-3+HS-15, TR-11+SB-10,
TR-12+HS-16, DP-8+GS-4, GS-16+SB-19, GS-17+SB-20. A few P4 entries bundle several small cleanups; the
appendices are the authoritative list.

---

## P1 — fix first

### F-01 A player who left keeps host powers — HS-1 (P1, reproduced)
`RequireHost` (`internal/handlers/utils/host.go:36-41`) checks that the caller's session names the game and
that `Players[session.SlotID].Host` is true, but **never that the slot's `UserID` is the caller**. The
host leaves (their session still says slot `p0`), a newcomer takes `p0` and becomes host, and the leaver
can now `kick` them or run any `layout` op — including `setScript` (see F-03). Reproduced end-to-end
with the real join/leave/kick handlers.
Fix: compare `slot.UserID == session.UserID` in `RequireHost`; the root fix is F-02.

### F-02 Kick and leave don't end membership — HS-2 (P1), carries forward prior B #5
`leave` (`leave/handler.go:49`) and `kick` (`kick/handler.go:62-70`) clear only the game slot. The
session keeps `GameID/SlotID/TeamID`, and `applyUpdate` (`repo/session/fields.go:30-43`) treats empty
strings as "unchanged", so nothing *can* clear them. A kicked player `reconnect`s with their token, gets
the old game's keyframe (`reconnect:71-80`) and every later delta, and Lua handlers
(`luahandler/handler.go:146-164`) and tic-tac-toe `move` still act for them without checking they
hold a slot.
Fix: a session operation that clears game membership (explicit, not via "empty means unchanged"), called
by leave/kick/join-elsewhere; every in-game action re-checks slot holdership. Already scoped as bug 2 of
`plans/session-resume.md` — this review raises it to P1.

### F-03 Client Lua sandbox escape: any game creator runs arbitrary JS on everyone who joins — CL-1 (P1, reproduced)
`client/layout/lua/host-api.ts:292-295` hands Lua a live JS object through fengari's `js` interop, so
`indri.state().constructor.constructor` is JS `Function`: `F(nil, "<any JS>")(nil)` executes. The
deny-list in `state.ts` (blocking `load`/`require`/`raw*`) doesn't matter once interop is reachable. The
author is the game host via `layout` `setScript` — and **any registered user becomes host by creating a
game** (`repo/game/operations.go:151`). Combined with F-01, even a *former* host qualifies.
CLAUDE.md's "residual risk: host-supplied content" section frames this as a server-deployment concern; it
is in fact a cross-user code-execution hole in every client.
Fix: do not expose `js` interop to scene scripts; pass state as plain Lua tables (copy in, copy out) and
close the global table. Until then, gate `setScript` to operator-authored scripts only.

## P2

| ID | Finding | Sources | Evidence / notes |
|---|---|---|---|
| F-04 | **Passwords and live session tokens are written to the server log** on any failed login/register/reconnect | SB-1, HS-3 | `router/router.go:16` logs raw bytes on decode failure, `:24` logs the decoded payload on any handler error. A failed `reconnect` logs a token valid for 7 days. |
| F-05 | Joining or creating a second game leaves a ghost player — still connected, possibly host — in the first | HS-5 | `join:62-85`, `create:64-87` overwrite `session.GameID` without leaving the old game. Same fix cluster as F-02. |
| F-06 | `reconnect` never marks the player connected again; a closing old connection marks a live user offline | HS-6, TR-13, HS-18 | `ConnectPlayer` has no production caller. Tracked as bugs 1 and 3 of `plans/session-resume.md` (epoch design). TR-13 unverified by the second pass but consistent with HS-6. |
| F-07 | `inquire` reports every team, and therefore every game, as full | HS-7 | Player slots are pre-declared up to the maximum (`operations.go:110-116`), so `inquire:138` always sees full teams; the reference client's join list (`client/components/join/join.tsx:25`) is therefore **empty**. |
| F-08 | Handler failures are never reported to the client | HS-8 | `router.go:22-25` only logs; `WriteError` is called only in `refresh`. A failed action gets no reply at all. |
| F-09 | Lua `indri.refresh()` / `refreshSelf()` bypass `ClientView` | HS-9, DP-4 | `GameService.Sanitize` (`services/game/game.go:170-190`) strips `privateData` only at the top five levels, so a nested key leaks; the payload also has the wrong shape and includes `data.layout`, and the reference client ignores it. Fix: send a real keyframe built from `events.ClientView`. |
| F-10 | Every Lua handler run deletes `null` placeholders and turns `[]` into `{}` in team and scene data | HS-4 (reproduced) | Breaks shape stability, so every Lua action re-keyframes the whole game. Only games with script Lua `handlers` are affected (neither shipped config has any). |
| F-11 | One Redis subscription error permanently stops all delta, keyframe, layout and kick fan-out on that instance | DP-2 | `services/events/redis.go:64-71` returns on the first receive error; the monitor and relay `return nil`, which the `errgroup` in `boot/serve.go:14-18` doesn't treat as failure, so HTTP keeps serving writes nobody sees. Multi-instance mode only. |
| F-12 | Graceful shutdown never records players as disconnected on persistent backends | SB-2 | The signal-cancelled ctx is baked into every store; the transport is closed after cancellation (`entrypoints/http.go:29-36`), so every disconnect write fails with `context canceled`. Players stay `connected: true` after every deploy. A crash has the same effect, so presence also needs reconciling at boot. |
| F-13 | Login never rotates the token or extends the session | SB-3 | `authentication.go:53-65` makes a new token, but every store's `New` returns the existing session unchanged and the new token is discarded. Re-login doesn't revoke a leaked token or reset the 7-day clock. Interacts with the 24 h idle-expiry decision (SB-5, P3). |
| F-14 | SQL backends scan the whole session/user table on hot paths | SB-6, overlaps DP-13 and prior A #9 | SQLite session and user `Find`, and Postgres session `Find("gameId")`, read and JSON-decode every row, and the session lookup runs on **every change event**. Mongo (the default) is unaffected. |
| F-15 | `runScript` / `runChunk` throw on non-string Lua errors, crashing the board for all viewers | CL-2 (reproduced; P1→P2) | `error({})` / `error(nil)` throw `TypeError` out of `host-api.ts:231` and `runtime.ts:83`; `board-view.tsx:99` calls it unguarded in an effect and there is no ErrorBoundary. Host-only trigger. |

## P3 — highlights

The full P3 list is in the appendices. The ones worth scheduling, grouped:

- **Transport robustness** — SSE and graphqlws writes have no deadline, so a non-reading client blocks kick
  and shutdown, and graphqlws can deadlock after 16 queued control frames (TR-3, borderline P2). Slow clients
  lose frames and stay connected (TR-2 = prior B #10; melody's 10 s write deadline narrows it on ws). WebRTC
  handles a message before the conn is registered (TR-1 = prior A #7). Same-host origin rule allows DNS
  rebinding (TR-6 = prior A #10, **still needs a decision**). No connection caps or pre-auth idle timeout
  (TR-9). WebRTC ignores the max message size (TR-7) and has a `peer.pc` data race (TR-10). Kick doesn't
  close a slow ws client (TR-4).
- **Config** — `INDRI_WS_PING_PERIOD=0` crashes the process on the first connection (TR-11, SB-10). One bad
  value half-applies the config and silently zeroes every later setting (SB-8). The script loader calls
  `log.Fatalf` inside a constructor that returns an error (SB-16).
- **Delta protocol edge cases** — ordering uses each instance's wall clock, so clock skew in multi-instance
  mode drops or reorders deltas; the per-game `Version` should reach the client (DP-1, GS-9; P2 in
  multi-instance deployments). Key changes inside array elements skip re-keyframe (DP-7). Keys containing `.`
  corrupt client state (DP-8, GS-4). Go and JS sort some non-ASCII keys differently (DP-9). The client delta
  buffer grows without bound and is fully re-sorted on every message (DP-10). No reconnect path, and deltas
  aren't discarded on a game change (DP-11). An older keyframe can overwrite a newer one (DP-12). One serial
  monitor loop fans out every game while writers hold the game lock (DP-13). A `__proto__` key makes the
  MessagePack client throw (DP-6). A joiner misses deltas between keyframe and session update (DP-3, HS-15).
- **Store divergence** — Mongo `$set` can't clear `omitempty` top-level fields such as `teams` (GS-2).
  `UpdateField` on an array index replaces the array with an object (GS-3). Mongo never returns
  `repo.ErrNotFound` (GS-5). Mongo hands handlers `bson.D`/`bson.A` while the other stores return maps and
  slices (GS-7). The memory store aliases non-JSON values (GS-6). Changing `code`/`id` diverges across stores
  (GS-8). `update()` makes private games public (GS-11, latent). A legacy ObjectID `_id` breaks `FindOpen`
  (GS-1, P2→P3; dev databases only, needs the release note from prior A #4). SQLite session `New` races
  under concurrent logins (SB-7). The SQL session `Update` rewrites the whole blob with no fence (SB-12 =
  prior A #5; becomes live once F-02 clears fields). SQLite has no `busy_timeout` (SB-14). The Postgres idle
  pool is 2 (SB-13).
- **Auth hardening** — user enumeration by response shape and bcrypt timing, and no rate limit (SB-11,
  HS-13). The login reply leaks `User.PrivateData` (HS-12). Mongo doesn't check expiry on read, and the agreed
  24 h idle expiry isn't implemented (SB-5). Changing the Mongo TTL will fail boot on existing databases
  (SB-9, P4 but a trap for session-resume).
- **Lua on the server** — no time or instruction limit, and the full stdlib (`os`, `io`, `debug`, `load`) is
  loaded (HS-11). Assigning an array to team/scene data panics the handler (HS-10). On the client, the
  instruction budget is bypassable via `pcall` (CL-6).
- **Layout validator split-brain** (CLAUDE.md requires Go and TS to match) — Go accepts absolute
  placements, unknown placement kinds, missing placements and unknown widget types that TS rejects, which
  blanks the board; TS accepts `z` that Go rejects (CL-3, P2→P3). Nested sub-grid rules diverge (CL-4). TS has
  no size or widget-count cap (CL-5).
- **Game / example** — `checkDiagonalWin` only detects full-length diagonals (CL-7). The root `config.json`
  lacks `turn`/`winningTeam` (CL-8). `create` leaves orphan games on failure and creation is unbounded
  (HS-14).
- **Ops** — the Docker image exits on start because its CMD passes no `-script` (SB-4, P2→P3; no compose or
  CI uses it). The image runs as root with the toolchain and an over-broad build context, and compose
  publishes unauthenticated Mongo and Redis on all interfaces (SB-15).
- **Docs** — several places still describe MongoDB-only storage (CL-9) and the old identity model (HS-26).
  `playerData` visibility is misdocumented (DP-14). `docs/tasks.md` is stale (CL-10).

## Prior findings carried forward

| Prior | Now | Status at 3eab208 |
|---|---|---|
| A #4 Mongo ObjectID→string, needs release note | GS-1 | open (P3) |
| A #5 whole-blob session Update | SB-12 | open (P3; latent until F-02) |
| A #7 WebRTC OnOpen/OnMessage race | TR-1 | open (P3) |
| A #9 SQLite Find full scan | F-14 | open (P2) |
| A #10 origin DNS rebinding | TR-6 | open, **needs a decision** |
| B #5 leave/kick don't clear session | F-02 | open (**P1**) |
| B #10 slow clients drop frames | TR-2 | open (P3; deferred to session-resume) |
| SQL pool not closed on constructor failure | SB-18 | open (P4) |
| `repo.ErrConflict` unused | GS-15 | open (P4) |
| `isPgDuplicateKey` ×3 | GS-14 | open (P4) |
| SQLite `busy_timeout` | SB-14 | open (P3) |
| Postgres redundant indexes | GS-16, SB-19 | open (P4, now verified) |
| Postgres NUL strings | GS-17, SB-20 | plausible, not run (no Postgres) |
| store wrapper duplication ×4 | GS-13 | open (P4) |
| `authResponse` ×2 | HS-20 | open (P4) |
| `SanitizeDelta` dead branch | DP-16 | open (P4; its privateData filtering is now redundant too) |
| one encode per recipient | TR-12, HS-16 | open (P3) |
| CI Postgres job never confirmed green | SB-17 | unknown |

Every other prior finding (all of Review 0, A #1/2/3/6/8, B #1/2/3/4/6/7/8/9) is re-verified **fixed**.

## Suggested remediation order

Each theme is small enough for one plan and one branch. Items already scoped in `plans/session-resume.md`
are marked (SR).

1. **Membership and identity** — F-01, F-02 (SR), F-05, F-06 (SR), F-13, SB-12, SB-5 (SR), DP-3,
   HS-24. One design: an explicit "leave game" session operation, slot-holder checks on every in-game action,
   and token rotation on login. Best done as, or folded into, the session-resume build.
2. **Client script trust boundary** — F-03, F-15, CL-6, DP-6, and the validator split-brain (CL-3/4/5).
   Needs a decision (below).
3. **Logging and error reporting** — F-04, F-08, F-11, F-12, SB-8, TR-11/SB-10, SB-16. Small, mechanical,
   high value; F-04 alone is a one-line fix.
4. **Lua on the server** — F-09, F-10, HS-10, HS-11.
5. **Lobby** — F-07 (joining is broken in the reference client).
6. **Transport robustness** — TR-1..TR-12 (TR-2 is SR).
7. **Store correctness and SQL performance** — F-14, GS-2..GS-8, GS-11, SB-7, SB-13, SB-14, then the P4
   duplication clean-ups.
8. **Delta protocol hardening** — DP-1 (send `Version`), DP-7..DP-13, GS-3/GS-4.
9. **Ops, CI, docs, tests** — SB-4, SB-15, SB-17 (add Mongo/Redis/Postgres services to CI), docs drift,
   and tests for every auth-path handler (HS-19).

## Decisions

1. **Scene-script trust model (F-03).** Decided by Boss 2026-09-28: **option (a)** — scene scripts stay
   host-authorable, and the client sandbox is rebuilt so scripts never receive a live JS object and
   fengari's `js` interop library is never reachable. State crosses the boundary as copied Lua tables.
   CL-2 (non-string errors throw), CL-6 (`pcall` bypasses the instruction budget) and DP-6 (`__proto__`)
   belong in the same plan, since the sandbox is only as strong as its weakest exposed function.
   Follow-up decision (Boss, 2026-09-28): arrays reach client scripts as **1-based** Lua tables, matching
   standard Lua and the server-side Lua handlers (`luahandler/runtime.go` `jsonToLua`). Today they are
   0-based only as a side effect of fengari-interop's JS proxies; the four shipped tic-tac-toe scene
   scripts are updated in the same plan.
2. **Origin rule (TR-6, prior A #10).** Decided by Boss 2026-09-28: **accept every origin by default, with
   an optional allowlist.** `INDRI_ALLOWED_ORIGINS` already exists; the change is that an empty value
   accepts all origins (today it rejects cross-origin browsers), and a non-empty value enforces the list.
   TR-6 is thereby accepted as a known default rather than a bug; the docs (`.env.example`,
   ARCHITECTURE.md, PROTOCOL.md) must say so. Follow-up (Boss, 2026-09-28): **when a list is set, it is
   the whole policy**: listed origins plus requests with no `Origin` header. The same-host rule, which
   let DNS rebinding through, is dropped, so a React Native iOS client lists its server's own origin.
3. **F-02.** Decided by Boss 2026-09-28: **fix on its own**, ahead of and separate from the session-resume
   build. F-01 and F-05 share its root cause and belong in the same fix.
