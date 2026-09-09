---
id: 022-e28e
title: Migrate tic-tac-toe to a data-driven layout
status: ready
priority: P2
type: feature
created: "2026-09-09T19:04:59.225Z"
updated: "2026-09-09T19:05:28.164Z"
dependencies: ["018", "021"]
plan: plans/layout-engine-renderer.md
plan_step: Step 12
depends_on: ["stories/018-d50f-pending-P2-text-image-and-sub-grid-widgets.md", "stories/021-9370-pending-P2-lua-to-websocket-bridge.md"]
---

# Migrate tic-tac-toe to a data-driven layout

## Problem Statement

app/index.tsx hardcodes a 3x3 board read from currentScene.data.board. Expressing it as a real layout proves the engine handles an actual game, and proves Lua works as the binding layer between authoritative game state and widget presentation rather than being a toy.

## Acceptance Criteria

- [ ] VERIFY: cd client && pnpm test
- [ ] config.json and example/tictactoe/config.json gain a data.layout that parses with zero issues
- [ ] The 9 sub-grid rects tile 3x3 without overlap, asserted by a test
- [ ] Every widget id referenced by the scene script exists in the layout, asserted by a test
- [ ] A scene Lua script maps data.board onto widget text on stateChanged, and sends a move action on widgetPress
- [ ] The Go move handler is unchanged and still writes data.board; the layout reads that state rather than replacing it
- [ ] The Lua 1-indexed to board 0-indexed offset is correct and covered, so the board does not silently transpose
- [ ] app/index.tsx contains zero hardcoded board markup
- [ ] [MANUAL] A full game is playable across two browser windows and one device, with both players seeing every move
- [ ] If the fengari spike failed on native, a declarative bind fallback is used instead and the chosen path is stated explicitly

## Files

- config.json
- example/tictactoe/config.json
- client/app/index.tsx
- client/layout/fixtures/tictactoe.json

## Proof

- [ ] [completeness] Completeness
- [ ] [feature-availability] Feature availability
- [ ] [robustness] Robustness
- [ ] [resilience] Resilience
- [ ] [security] Security
- [ ] [defense-in-depth] Defense in depth
- [ ] [input-validation] Input validation
- [ ] [thread-safety] Thread safety
- [ ] [configurability] Configurability

## Work Log

