// Benchmark results (Node 22, MacBook M-series, 2026-09-15):
// 200 widgets, 1 delta:    ~0.21ms
// 200 widgets, 10 deltas:  ~0.19ms
// 200 widgets, 100 deltas: ~0.29ms
// 200 widgets, 500 deltas: ~0.49ms
//
// Recommendation: The current deep-clone-and-replay approach is fast enough at this scale.
// Even at 500 accumulated deltas the cost is under 0.5ms per message. Optimisation
// (e.g. folding settled deltas into the base on a timer) is not needed at PoC stage.
// If widget counts exceed ~1000 or delta accumulation grows past ~1000 between keyframes,
// revisit — but the data here does not justify preemptive work.

import test from "node:test"
import assert from "node:assert/strict"
import {performance} from "node:perf_hooks"

import {GameStateParser} from "./game-state-parser.ts"
import type {UpdateMessage} from "../models/models.ts"

interface Game {
    code: string
    stage: {
        currentScene: string
        sceneOrder: string[]
        scenes: Record<string, unknown>
    }
    data: Record<string, unknown>
}

function makeGame(widgetCount: number): Game {
    const cols = Math.ceil(Math.sqrt(widgetCount))
    const widgets: Record<string, unknown> = {}
    for (let i = 0; i < widgetCount; i++) {
        const col = i % cols
        const row = Math.floor(i / cols)
        widgets[`w${i}`] = {
            type: "text",
            placement: {kind: "grid", col, row, w: 1, h: 1},
            config: {text: `Widget ${i}`},
        }
    }
    return {
        code: "BENCH",
        stage: {currentScene: "main", sceneOrder: ["main"], scenes: {}},
        data: {
            layout: {
                grid: {cols, rows: cols},
                scenes: {
                    main: {widgets},
                },
            },
        },
    }
}

function makeUpdateMessage(ts: number, widgetIdx: number): UpdateMessage {
    return {
        op: "update",
        ts: new Date(ts).toISOString(),
        updated: {[`data.layout.scenes.main.widgets.w${widgetIdx}.config.text`]: `Updated ${widgetIdx}`},
    } as UpdateMessage
}

for (const n of [1, 10, 100, 500]) {
    test(`delta replay: 200 widgets, ${n} deltas`, () => {
        const parser = new GameStateParser<unknown>()
        parser.set(makeGame(200), new Date(0))

        // Accumulate n deltas
        for (let i = 0; i < n; i++) {
            parser.update(makeUpdateMessage(i + 1, i % 200))
        }

        // Time the cost of one more update which triggers reapply over n accumulated deltas
        const start = performance.now()
        parser.update(makeUpdateMessage(n + 1, 0))
        const elapsed = performance.now() - start

        // Assert only that it completes. The numbers are the deliverable.
        assert.ok(elapsed < 10_000, `reapply over ${n} deltas took ${elapsed.toFixed(1)}ms (should complete)`)

        console.log(`[bench] 200 widgets, ${n} deltas: ${elapsed.toFixed(2)}ms`)
    })
}
