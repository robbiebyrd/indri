---
id: 022-e28e
title: Migrate tic-tac-toe to a data-driven layout
status: complete
priority: P2
type: feature
created: "2026-09-09T19:04:59.225Z"
updated: "2026-09-12T01:10:14.398Z"
dependencies: ["018", "024"]
plan: plans/layout-engine-renderer.md
plan_step: Step 12
depends_on: ["stories/018-d50f-ready-P2-text-image-and-sub-grid-widgets.md", "stories/024-8ef7-ready-P2-lua-outbound-send-and-state-change-observation.md"]
started_at: "2026-09-12T00:53:04.000Z"
completed_at: "2026-09-12T01:10:14.398Z"
---

# Migrate tic-tac-toe to a data-driven layout

## Problem Statement

app/index.tsx hardcodes a 3x3 board read from currentScene.data.board. Expressing it as a real layout proves the engine handles an actual game, and proves Lua works as the binding layer between authoritative game state and widget presentation rather than being a toy.

## Acceptance Criteria

- [x] VERIFY: cd client && pnpm test
- [x] config.json and example/tictactoe/config.json gain a data.layout that parses with zero issues
- [x] The 9 sub-grid rects tile 3x3 without overlap, asserted by a test
- [x] Every widget id referenced by the scene script exists in the layout, asserted by a test
- [x] A scene Lua script maps data.board onto widget text on stateChanged, and sends a move action on widgetPress
- [x] The Go move handler is unchanged and still writes data.board; the layout reads that state rather than replacing it
- [x] The Lua 1-indexed to board 0-indexed offset is correct and covered, so the board does not silently transpose
- [x] app/index.tsx contains zero hardcoded board markup
- [REJECTED] [MANUAL] A full game is playable across two browser windows and one device, with both players seeing every move ([MANUAL] The press->send->server->delta->repaint loop is proven in Node against the real script and real config, but never against a live server. Needs a human to play a full game across two browser windows and one device.)
- [x] If the fengari spike failed on native, a declarative bind fallback is used instead and the chosen path is stated explicitly

## Files

- config.json
- example/tictactoe/config.json
- client/app/index.tsx
- client/layout/fixtures/tictactoe.json

## Proof

- [x] [completeness] Completeness (9 of 10 criteria checked; criterion 9 is [MANUAL] live-play and explicitly rejected. 207/207 tests, typecheck clean, lint down to 6 warnings from 7.)
- [x] [feature-availability] Feature availability (CRITICAL: found and fixed the missing lua_checkstack that made the Lua feature dead on any realistically-sized state. Mutation-verified - disabling the two reserve（） calls yields 5 failures with 'Error: stack overflow'. Prior stories' tests passed only because their fixtures were a toy 1-widget layout.)
- [x] [robustness] Robustness (The scene script guards its widget id match with a pattern, because a scene handler also sees non-cell widgets like title and would otherwise send move='i,t'. A subgrid is not pressable, so a press on a cell cannot also report a press on its container.)
- [x] [resilience] Resilience (Both shipped config.json files parse with ZERO issues - warnings counted, not just errors - with sceneIds cross-checked against stage.scenes, and a deep-equal so the two configs cannot drift.)
- [x] [security] Security (The Go move handler is unchanged and still owns all game logic; internal/ untouched. The layout READS data.board rather than replacing it, so authority stays server-side.)
- [x] [defense-in-depth] Defense in depth (Widget ids the script writes to are derived by RUNNING the script and reading the override layer, since ids are built by string.format and nothing static could see them.)
- [x] [input-validation] Input validation (Cell coverage is proven by a map asserting every sub-grid cell is claimed exactly once, which catches gaps as well as overlaps - pairwise collision alone would not. The 1-indexed/0-indexed mapping is mutation-verified: transposing it in the shipped config fails the test.)
- [~] [thread-safety] Thread safety (Single-threaded React plus one synchronous Lua state.)
- [x] [configurability] Configurability (The layout lives in config.json, so the board is now data. Lua is the binding layer with no declarative expression language, which is the ADR this story exists to prove.)

## QA

- [ ] 207/207 tests, typecheck clean, lint 7 -> 6 warnings, web bundle exports
- [ ] Missing lua_checkstack found and fixed; mutation-verified with 5 failures
- [ ] [MANUAL] live two-window + device play still needed

## Work Log

### 2026-09-12T01:09:58.471Z - Tic-tac-toe now renders through the engine, and this story also closed the gap 024 left: useLuaBridge/useOverrides had NO consumer, so the whole host API was inert. BoardView now renders mergeOverrides(parseLayout(raw), overrides) in a second useMemo keyed on [server, overrides], so a script painting one cell re-merges without re-validating; merge is structure-sharing so WidgetHost's identity memo still bails on untouched subtrees. Press path uses a WidgetPressContext rather than a prop, because the path runs board -> scene -> sub-grid -> widget and a prop would have to be re-threaded by every container including future widget types. A subgrid is deliberately NOT pressable - it is a coordinate space, and nesting press surfaces would make a press on c01 also report a press on cells (DOM click bubbles on web). sceneChanged was added and owned by the bridge, firing AFTER stateChanged so a handler told 'the scene is now X' reads a state that already says X. CRITICAL BUG FOUND AND FIXED, outside the listed files: pushReadOnly/tableToJs in host-api.ts never called lua_checkstack. Lua reserves only LUA_MINSTACK (20) slots per host-function entry and does NOT grow the stack on its own, so fengari threw a bare Error('stack overflow') with no traceback. Any realistically-sized game state blew up - THE ENTIRE LUA FEATURE WAS DEAD ON REAL DATA, and every green test in 019/020/024 passed only because their fixtures were a toy 1-widget layout. I independently mutation-verified the fix: disabling the two reserve() calls produces 5 failures with that exact error. Layout detail worth knowing: MIN_DIM (8) applies to sub-grids too, so a literal 3x3 sub-grid fails GridSizeSchema; the cells sub-grid is 9x9 with nine 3x3 cells. The scene script guards its widget id match with ^c(%d)(%d)$ because a scene handler also sees title, and an unguarded script would send move='i,t'. Go move handler untouched; internal/ untouched. 9 new tests, 207 total, lint down from 7 warnings to 6.


### 2026-09-12T01:10:13.653Z - Proof completeness set PROVEN: 9 of 10 criteria checked; criterion 9 is [MANUAL] live-play and explicitly rejected. 207/207 tests, typecheck clean, lint down to 6 warnings from 7.

### 2026-09-12T01:10:13.735Z - Proof feature-availability set PROVEN: CRITICAL: found and fixed the missing lua_checkstack that made the Lua feature dead on any realistically-sized state. Mutation-verified - disabling the two reserve() calls yields 5 failures with 'Error: stack overflow'. Prior stories' tests passed only because their fixtures were a toy 1-widget layout.

### 2026-09-12T01:10:13.815Z - Proof robustness set PROVEN: The scene script guards its widget id match with a pattern, because a scene handler also sees non-cell widgets like title and would otherwise send move='i,t'. A subgrid is not pressable, so a press on a cell cannot also report a press on its container.

### 2026-09-12T01:10:13.895Z - Proof resilience set PROVEN: Both shipped config.json files parse with ZERO issues - warnings counted, not just errors - with sceneIds cross-checked against stage.scenes, and a deep-equal so the two configs cannot drift.

### 2026-09-12T01:10:13.972Z - Proof security set PROVEN: The Go move handler is unchanged and still owns all game logic; internal/ untouched. The layout READS data.board rather than replacing it, so authority stays server-side.

### 2026-09-12T01:10:14.053Z - Proof input-validation set PROVEN: Cell coverage is proven by a map asserting every sub-grid cell is claimed exactly once, which catches gaps as well as overlaps - pairwise collision alone would not. The 1-indexed/0-indexed mapping is mutation-verified: transposing it in the shipped config fails the test.

### 2026-09-12T01:10:14.133Z - Proof defense-in-depth set PROVEN: Widget ids the script writes to are derived by RUNNING the script and reading the override layer, since ids are built by string.format and nothing static could see them.

### 2026-09-12T01:10:14.210Z - Proof thread-safety set NOT_APPLICABLE: Single-threaded React plus one synchronous Lua state.

### 2026-09-12T01:10:14.294Z - Proof configurability set PROVEN: The layout lives in config.json, so the board is now data. Lua is the binding layer with no declarative expression language, which is the ADR this story exists to prove.
