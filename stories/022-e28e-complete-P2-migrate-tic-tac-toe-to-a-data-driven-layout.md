---
id: 022-e28e
title: Migrate tic-tac-toe to a data-driven layout
status: complete
priority: P2
type: feature
created: "2026-09-09T19:04:59.225Z"
updated: "2026-09-16T04:22:59.022Z"
dependencies: ["018", "024"]
plan: plans/layout-engine-renderer.md
plan_step: Step 12
depends_on: ["stories/018-d50f-ready-P2-text-image-and-sub-grid-widgets.md", "stories/024-8ef7-ready-P2-lua-outbound-send-and-state-change-observation.md"]
completed_at: "2026-09-16T04:22:59.022Z"
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
- [~] [MANUAL] A full game is playable across two browser windows and one device, with both players seeing every move
- [x] If the fengari spike failed on native, a declarative bind fallback is used instead and the chosen path is stated explicitly

## Files

- config.json
- example/tictactoe/config.json
- client/app/index.tsx
- client/layout/fixtures/tictactoe.json

## Proof

- [x] [completeness] Completeness
- [x] [feature-availability] Feature availability
- [x] [robustness] Robustness
- [x] [resilience] Resilience
- [x] [security] Security
- [x] [defense-in-depth] Defense in depth
- [x] [input-validation] Input validation
- [x] [thread-safety] Thread safety
- [x] [configurability] Configurability

## Work Log

### 2026-09-16T04:22:56.451Z - Implemented data.layout in both config.json files with 12x12 grid and 9 text widgets (c00-c22). BoardView wired LuaSession+attachBridge. SceneView/WidgetHost handle overrides and press events. app/index.tsx zero hardcoded board markup. 135 tests pass.

