# Review slice: client layout system, server layout validator, tictactoe, docs

Worktree: `/Users/robbiebyrd/Projects/indri/.worktrees/codebase-review` (branch `wip/codebase-review`, base `main` 3eab208).
Read-only. All line numbers are from this worktree.

## Environment / test runs

- `go test -race ./internal/handlers/actions/layout/... ./example/...` → **PASS** (layout, tictactoe move/restart/board all ok). Needed `GOCACHE` pointed at the scratchpad; the default cache dir is outside the sandbox.
- Client `node_modules` is absent in the worktree but present in the main checkout at `/Users/robbiebyrd/Projects/indri/client/node_modules`. I ran the client checks there (same commit content):
  - `pnpm test` → **249 pass / 0 fail** (8 suites).
  - `pnpm run typecheck` → **clean**.
  - `pnpm run lint` → **0 errors, 7 warnings** (missing-dep useEffect in app/index.tsx and join.tsx, two unused vars + one `==` in loggedIn.tsx, one unused eslint-disable in board-view.tsx, one named-as-default import in create.tsx). All pre-existing nits.
- Sandbox-escape / budget / non-throw experiments were run against the real fengari in the main checkout with `node --experimental-strip-types`. Results are quoted inline below.

---

## Prior findings re-check

The prior file (`reviews/2026-09-28-prior-reviews-recovered.md`) is a recovery of branch-diff reviews and the lost 2026-09-09 whole-codebase review. Items that touch this slice:

- **Review B #2 — layout host check by userId vs slot (handler.go:57).** Status claimed **FIXED** (0f85d3b, shared `handlerUtils.RequireHost`). **CONFIRMED FIXED.** `internal/handlers/actions/layout/handler.go:31` calls `handlerUtils.RequireHost`, which resolves the caller from the connection's own `sessionId` and checks `g.Players[slot].Host` (utils `RequireHost` lines 16-45). No client-supplied userId is trusted. Good.
- **Review 0 / BUG-1 — tictactoe move through `Store.Mutate`.** Status **FIXED** (9a1a338). **CONFIRMED.** `example/tictactoe/server/handlers/move/handler.go:69` does the whole move (validate → board write → win check → turn flip) inside one `GameRepo.Mutate`. Restart (`restart/handler.go:50`) likewise. No lost-update window.
- **CLAUDE.md / PROTOCOL.md "residual risk: host-supplied Lua and image URIs, not sandboxed."** This is documented as a *deploy-time* residual risk for the *server* (`setScript` stores Lua verbatim). What the docs do **not** capture is that the *client* ships an explicit Lua sandbox (`client/layout/lua/state.ts`) presented as a security boundary, and that boundary is **defeated at runtime** — see CL-1. So the prior "documented, accepted" framing under-states the client exposure.

No prior finding in this slice regressed.

---

### [P1] Client Lua sandbox is fully escapable — host script runs arbitrary JS in every viewer's browser
- ID: CL-1
- Category: security
- Location: `client/layout/lua/host-api.ts:263` (`installHostApi` opens the `js` interop lib), `:292-297` (`indri.state()` pushes a JS object via `fengariInterop.push`); sandbox intent in `client/layout/lua/state.ts:37-48`; call site `client/components/board/board-view.tsx:99` (`session.runScript(scene.script, …)`)
- Confidence: CONFIRMED
- Prior: partially covered (docs mention server-side `setScript` risk; the client-sandbox bypass is new)
- What: `state.ts` deliberately denies `load`/`require`/`raw*` etc. to stop a script from breaking out of Lua. But `installHostApi` calls `luaL_requiref(L, "js", fengariInterop.luaopen_js, …)` and `indri.state()` returns a JS object pushed with `fengariInterop.push`. fengari-interop wraps JS values as userdata whose `__index` proxies real property access, so a script can walk the JS prototype chain: `indri.state().constructor.constructor` is the JS `Function` constructor, which compiles and runs arbitrary JavaScript. The deny-list is irrelevant once a live JS object is reachable.
- Failure scenario: A game host sends `layout`/`setScript` with `source = "local F = indri.state().constructor.constructor; F('globalThis.PWNED=1; /* fetch(...) exfiltrate, rewrite DOM, etc */')()"`. Every player who opens that game's board runs `board-view.tsx` → `session.runScript(scene.script)` → the script executes arbitrary JS in each player's browser tab (web build) or JS context (native). Verified: my probe printed `call=true 42.0` and `PWNED=yes`, and a one-liner `st.constructor.constructor("globalThis.PWNED='yes'; return typeof process")()` set the global. This is stored-XSS / RCE-in-page reachable by any host against all players.
- Suggested fix: Do not return live JS objects to Lua. Marshal `indri.state()` into a Lua table (there is already `luaTableToJs` for the reverse; add a JS→Lua table builder) instead of `fengariInterop.push`, and do not open `luaopen_js` in a session that runs untrusted scripts. At minimum, gate script execution behind a trust decision and document that the client sandbox is not a boundary today (it currently reads as one).

### [P1] `runScript` / `runChunk` break their "never throws" contract on non-string Lua errors, crashing the board render
- ID: CL-2
- Category: bug
- Location: `client/layout/lua/host-api.ts:220-239` (`runScript`), `client/layout/lua/runtime.ts:63-88` (`runChunk`); unguarded call at `client/components/board/board-view.tsx:99-102`
- Confidence: CONFIRMED
- Prior: new
- What: Both functions document "Never throws … errors surface as {ok:false}". They extract the Lua error with `to_jsstring(lua.lua_tostring(L, -1))`. When the pcall error value is not a string/number (`error({})`, `error(nil)`), `lua_tostring` returns `null` and `to_jsstring(null)` throws `TypeError: to_jsstring expects a Uint8Array`, which propagates out of `runScript`. Verified: `error({})` and `error(nil)` both produce `THREW TypeError` instead of `{ok:false}`.
- Failure scenario: A host `setScript` with `source = "error({})"`. `board-view.tsx:99` calls `session.runScript` with no try/catch inside a `useEffect`; the throw becomes an uncaught error during render/effect, breaking the board for every viewer (a blank/crashed board is exactly the "worse failure" the code says it avoids). Same class of throw is reachable through `indri.log` with a cyclic table (`luaTableToJs` recurses to stack-overflow, thrown out of the cfunction).
- Suggested fix: In `runScript`/`runChunk`, guard the error extraction: `const raw = lua.lua_tostring(L,-1); const error = raw ? to_jsstring(raw) : "(non-string Lua error)"`. Also wrap the `runScript` call in `board-view.tsx` in try/catch as defence in depth.

### [P2] Validator divergence: server accepts absolute placements / unknown placement kinds that the client schema rejects, blanking the board for everyone
- ID: CL-3
- Category: security
- Location: Go `internal/handlers/actions/layout/validate.go:170-181` (`checkAbsolutePlacement`) and `:88-107` (switch only handles `grid`/`absolute`); TS `client/layout/schema/placement.ts:5-26` (`Percent`, discriminated union) via `client/layout/schema/layout.ts:181` (`GameLayoutSchema.safeParse`)
- Confidence: CONFIRMED
- Prior: new
- What: The two validators are required by CLAUDE.md to enforce identical rules. For placements they do not:
  - Absolute: Go only rejects *numeric* values (must not be a pixel number). It does **not** require `left/top/width/height` to be present, does not check the `%` format, and does not reject extra keys. TS requires all four as strict `^-?\d+(\.\d+)?%$` strings.
  - Kind: Go's switch silently ignores any `placement.kind` that is neither `grid` nor `absolute` (no validation, accepted). TS `Placement` is a discriminated union on `kind` → schema failure.
- Failure scenario: A host sends `addWidget` with `placement: {"kind":"absolute","left":"abc","top":"1%","width":"1%","height":"1%"}` (or `{"kind":"foo"}`, or absolute with missing fields). Go `ValidateLayout` accepts it and `Mutate` commits it; the layout frame broadcasts to all clients. On every client, `parseLayout` runs `GameLayoutSchema.safeParse`, which fails the whole document, so `parseLayout` returns `{issues}` with **no** `layout` → `board-view.tsx:73` `scene` is falsy → blank board for all players until the layout is fixed. A host can brick the board with one structurally-valid (per server) write.
- Suggested fix: Make Go reject unknown `placement.kind`, require the four percentage fields for `absolute`, and validate the `%` format, matching `placement.ts`. Add a `store_contract`-style shared fixture list exercised by both validators.

### [P2] Validator divergence: server validates/reject widgets nested inside sub-grids; client validates neither their overlap nor their bounds
- ID: CL-4
- Category: bug
- Location: Go `internal/handlers/actions/layout/validate.go:109-134` (recurses into `config.widgets`, runs overlap+bounds on nested widgets); TS `client/layout/schema/layout.ts:196-208` (`findOverlaps`/`findOutOfBounds` run only on top-level `scene.widgets`, never recurse)
- Confidence: CONFIRMED
- Prior: new
- What: Go's `validateSceneWidgets` recurses into every `subgrid` widget's `config.widgets` and runs the overlap check and bounds check on the nested widgets too. The TS validator's overlap and out-of-bounds passes only iterate the scene's top-level widgets — nested sub-grid children are never checked. (Separately, Go bounds-checks nested children against the **root** grid dims, not the sub-grid's own `config.grid`, which is itself likely wrong, but the divergence with TS is the split-brain CLAUDE.md warns about.)
- Failure scenario: Host adds a subgrid whose two children occupy the same cell (`a` and `b` both `{kind:grid,col:0,row:0,w:1,h:1}`). Go rejects the write with "widgets a and b overlap"; the client `parseLayout` would have accepted it with no issue. Any layout the host builds and tests in the client editor that has overlapping nested widgets is silently un-writable, with a server error the client never predicts. Conversely nested out-of-bounds children pass the client but are checked (against the wrong grid) by the server.
- Suggested fix: Make the TS validator recurse into `subgrid` `config.widgets` for both overlap and bounds, using each sub-grid's own `config.grid` for bounds — and fix Go to bound nested children against the sub-grid's own grid, not the root grid.

### [P3] Validator divergence: client enforces no size cap, no widget-count cap
- ID: CL-5
- Category: maintainability
- Location: Go `internal/handlers/actions/layout/validate.go:34-51` (`maxBytes` 256 KiB, `maxWidgets` 300); TS `client/layout/schema/layout.ts` (neither cap present)
- Confidence: CONFIRMED
- Prior: new
- What: Go rejects layouts over 256 KiB serialized or over 300 total widgets (including nested). The TS `parseLayout` has no equivalent, so the "deliberate duplicated pair" diverges on two of the documented structural rules. Practical impact is low because the server is the write authority (a client can never persist an over-cap layout), but it is a real drift and the client will happily render whatever it is handed. Worth either adding the caps to TS or documenting that these two rules are server-only.
- Failure scenario: N/A at runtime (server gate holds); this is drift against the CLAUDE.md contract that both validators enforce the same rules.
- Suggested fix: Add byte-size and widget-count checks to `parseLayout`, or amend CLAUDE.md/ARCHITECTURE.md to state these two limits are server-enforced only.

### [P3] Instruction budget is per-chunk cumulative and fully bypassable via pcall; docs oversell it
- ID: CL-6
- Category: security
- Location: `client/layout/lua/runtime.ts:16-48` (`INSTRUCTION_BUDGET`, `installBudgetHook`)
- Confidence: CONFIRMED
- Prior: new
- What: The count hook is installed once per state and never reset between `runScript` calls, so the "budget" is a lifetime counter for the whole session, not a per-script limit: several small scripts cumulatively trip it (verified: 10 loops of 150k instructions → "instruction budget exceeded" at run 3), and a legitimate long-lived board eventually dies. More importantly, the hook raises a catchable `luaL_error`, so a script wraps its infinite loop in `pcall` and simply resumes: `for i=1,3 do pcall(function() while true do end end) end` returned `survived 3 budget errors` (each iteration burns a full budget but never stops the script). The code's own comment concedes it is "best-effort," but combined with CL-1 there is effectively no CPU guard on host scripts, and no memory guard at all (`string.rep("a", 2^27)` allocated 128 MB in 1.3 s; a pathological `string.find` pattern ran 4.2 s of host-side backtracking that the count hook cannot interrupt).
- Failure scenario: Malicious host script pins a viewer's CPU/RAM (`while true do pcall(...) end`, or a catastrophic `string.find` pattern, or repeated `string.rep`). The budget does not stop it.
- Suggested fix: Reset the hook count between top-level `runScript` invocations; make the abort non-catchable (e.g. set a hard "aborted" flag the host checks and refuses to re-enter, or run scripts in a Web Worker with a wall-clock kill). Treat CPU/memory limits as unsolved for untrusted scripts and gate script execution on trust (ties to CL-1).

### [P3] tictactoe `checkDiagonalWin` only detects full-length diagonals, using `count == rows`
- ID: CL-7
- Category: bug
- Location: `example/tictactoe/server/handlers/move/handler.go:232-267`
- Confidence: CONFIRMED (latent; correct for the shipped 3×3 board)
- Prior: new
- What: `checkDiagonalWin` counts marks along each diagonal offset and declares a win when `count == rows`. That is only correct for a square board where the win length equals `rows`. For any non-square board (`rows != cols`) the two main diagonals have length `min(rows,cols)`, so `count == rows` can never be true on the shorter axis and diagonals are silently never winnable; on a board taller than wide it can also mis-count. The board is fixed at 3×3 in both config files, so no live bug today, but `getBoardSize`/`buildEmptyBoard` are written generically as if other sizes were supported.
- Failure scenario: Change the script board to 4×3 (or any non-square) and diagonal wins stop being detected while straight-line wins still work — an inconsistent, hard-to-spot rules bug.
- Suggested fix: Track the required run-length explicitly (e.g. `min(rows, cols)` or a configured win length) instead of `count == rows`, or document that the handler only supports square boards.

### [P3] Root `config.json` omits `turn`/`winningTeam`, inconsistent with `example/tictactoe/config.json` and the move handler's expectations
- ID: CL-8
- Category: docs
- Location: `/config.json:8-24` (teams have no `turn`), `:33-51` (scene data has no `winningTeam`), vs `example/tictactoe/config.json:14,23,41` and README.md:45 (`go run ./cmd/server -script ./config.json`)
- Confidence: CONFIRMED
- Prior: new
- What: The repo-root `config.json` that README/CLAUDE point `cmd/server` at is a tic-tac-toe-shaped script but is missing the `turn` team flags and the `winningTeam` scene field, and its layout script lacks the `result`/`restart` widgets and the `widgetPress` handler that the example config has. `cmd/server` doesn't register the `move`/`restart` handlers, so it can't actually play — but the file reads as a playable tic-tac-toe script and diverges from the real one. If a reader copies it to drive the example, `move` fails immediately with "turn is nil" (`move/handler.go:77-82`).
- Failure scenario: Following README to run `example/tictactoe` but passing the root `-script ./config.json` (natural given README shows that path for `cmd/server`) yields a game where every move errors "turn is nil".
- Suggested fix: Either make root `config.json` a minimal generic framework sample (clearly not tic-tac-toe) or bring it in line with `example/tictactoe/config.json`. Clarify in README which script each binary expects.

### [P3] Docs still describe the store layer as MongoDB-only in several places
- ID: CL-9
- Category: docs
- Location: `docs/ARCHITECTURE.md:26` ("`internal/repo/*` MongoDB stores"), `:28` ("writes ──► MongoDB"), `:146` ("`repo/game` is split by concern: `game.go` … `player.go`, `team.go`, `host.go`")
- Confidence: CONFIRMED
- Prior: new
- What: The request-lifecycle diagram labels the repo layer "MongoDB stores" and the write arrow "MongoDB", but the codebase now has four backends (`mongo*.go`, `memory*.go`, `sqlite*.go`, `postgres*.go`) selected by `INDRI_DB_BACKEND`, which the same doc describes correctly later (§Database backends). The file-split claim at :146 is also stale: `repo/game` has no `team.go`; the shared write logic lives in `operations.go` (and per-store `*_host.go`/`*_player.go`), which the doc doesn't mention. `internal/handlers/messages` auth.go is described as "unused stub" (:123) — it exists; not re-verified as unused.
- Failure scenario: N/A (documentation accuracy).
- Suggested fix: Change the diagram labels to "game/user/session stores (per `INDRI_DB_BACKEND`)" and update the `repo/game` file list to `game.go`, `operations.go`, `*_player.go`, `*_host.go`, `changes.go`, `interface.go`.

### [P3] `docs/tasks.md` is stale for this slice
- ID: CL-10
- Category: docs
- Location: `docs/tasks.md:40` (item 24 "Align environment variable names in code with those in .env.example")
- Confidence: PLAUSIBLE
- Prior: new (CLAUDE.md already flags tasks.md as aspirational)
- What: `.env.example` and `internal/repo/env/env.go` env keys line up (`LISTEN_ADDRESS`, `TRANSPORTS`, `DB_BACKEND`, `WS_*`, `WEBRTC_*`, `GRAPHQL_INIT_TIMEOUT`, etc.), so task 24 appears already done. Two `.env.example`-only keys exist by design (`INDRI_MONGO_PORT`, `INDRI_REDIS_EMPTY_PASSWORD`) — they are docker-compose variables, not `Vars` fields, which is correct but undocumented as such. CLAUDE.md already warns tasks.md is a backlog, not current state; flagging item 24 specifically.
- Suggested fix: Tick or drop tasks.md item 24; optionally note in `.env.example` that `INDRI_MONGO_PORT`/`INDRI_REDIS_EMPTY_PASSWORD` are compose-only.

### [P4] `indri.state()` returns a deep-frozen live object but relies on JSON clone; `widget().setStyle` from Lua is a dead no-op
- ID: CL-11
- Category: maintainability
- Location: `client/components/board/board-view.tsx:47-56` (`installWidgetGlobal` setStyle returns 0, does nothing), `:36-42` (setConfig only ever forwards a `text` field)
- Confidence: CONFIRMED
- Prior: new
- What: The client-side `widget(id).setStyle(...)` Lua binding is a documented no-op ("not yet consumed by the render layer"), and `widget(id).setConfig(patch)` only extracts `patch.text` — any other config key a script sets is silently dropped. This is fine for the tic-tac-toe demo (text-only) but is a partial/placeholder implementation that will silently ignore config on any other widget type. Note this is a *second*, separate widget binding from `LuaSession.widget()` in host-api.ts (which stores full style/config into the OverrideMap); the board-view one shadows it with `lua_setglobal("widget", …)`. Two overlapping implementations of the same global is confusing and DRY-violating.
- Failure scenario: A host script `widget("img").setConfig({uri="…"})` or `widget("x").setStyle({backgroundColor="red"})` appears to work but changes nothing.
- Suggested fix: Either route the `widget` global through `session.widget(id)` (host-api.ts already implements full setConfig/setStyle into OverrideMap and SceneView reads `overrides.widgets[id].config/style`) and delete the board-view reimplementation, or finish the render wiring and drop the "text-only" shortcut.

### [P4] `app/board/spike.tsx` is committed throwaway code
- ID: CL-12
- Category: maintainability
- Location: `client/app/board/spike.tsx:1-3` (header: "THROWAWAY — story 012 … Delete once the spike is signed off")
- Confidence: CONFIRMED
- Prior: new
- What: A self-described throwaway spike screen is still in the tree and reachable as an expo-router route (`/board/spike`). It duplicates sandbox-probe logic and imports `createSandboxedState` directly. Dead/aspirational code per its own comment.
- Failure scenario: N/A (cleanup); minor risk that a throwaway route ships in a build.
- Suggested fix: Delete `client/app/board/spike.tsx` now that the real runtime exists.

### [P4] Percent regex allows negative percentages for width/height in absolute placement
- ID: CL-13
- Category: bug
- Location: `client/layout/schema/placement.ts:5-7` (`/^-?\d+(\.\d+)?%$/`, applied to `width`/`height` too)
- Confidence: PLAUSIBLE
- Prior: new
- What: The `Percent` type permits a leading `-`, which is meaningful for `left`/`top` but nonsensical for `width`/`height`. A widget with `width:"-50%"` passes schema (and passes the Go validator, which only forbids numbers). RN will clamp/ignore it, so no crash, but it's an accepted-invalid value on both sides.
- Failure scenario: `addWidget` absolute with `width:"-50%"` renders a zero/degenerate box rather than being rejected.
- Suggested fix: Split into a signed percent (position) and non-negative percent (size), and mirror in Go.

---

## Notes on things checked and found OK

- `parseLayout` itself is non-throwing on malformed input (privateData walk, `safeParse`, semantic passes are all guarded); the 328-line test suite covers prototype-pollution-style keys, missing fields, etc. The throw risk is in the Lua path (CL-2), not `parseLayout`.
- `privateData` reserved-key rejection is present and equivalent on both sides (Go `checkPrivateData` recurses maps and slices; TS `collectKeys`/`findPrivateDataKeys` recurses all objects/arrays). No divergence found there.
- Grid bounds `[8,4096]` and the AABB overlap test (strict `<` on all four edges) match exactly between `validate.go:21-24`/`extractGrid` and `collision.ts:7-10`/`coords.ts` `MIN_DIM/MAX_DIM`.
- tictactoe move handler correctly resolves the caller from their own session (`gameAndTeamFromSession`), rejects out-of-turn (`turn` flag), moves after win (`sceneHasWinner`), occupied cells ("spot is taken"), out-of-bounds and malformed moves, and "session not in a game/team". The whole move is one `Mutate`, so concurrent moves are fenced. No move-out-of-turn / after-win / occupied-cell hole found.
- `firstFree` is correctly bounded to 64×64; `moveWidget` runs `canPlace` as UX-only and treats the server as authority.
