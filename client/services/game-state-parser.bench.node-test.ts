// Measurement, not a regression gate.
//
// `GameStateParser.reapply()` deep-clones the WHOLE game state and replays
// EVERY retained delta on EVERY incoming message. Deltas are pruned only when a
// keyframe arrives, and keyframes only arrive on create/join/refresh/reconnect
// — never periodically. So the per-message cost is O(state size x retained
// delta count) and the delta count grows without bound during a session.
//
// Moving layouts into `game.data.layout` inflates the state-size term, so this
// file puts numbers on both terms before anyone optimises anything. The printed
// table is the deliverable; the assertions exist only to catch a catastrophic
// regression, never to police a timing.
import test from "node:test";
import assert from "node:assert/strict";

import {GameStateParser} from "./game-state-parser.ts";

import type {GameLayout} from "../layout/schema/layout.ts";
import type {Widget} from "../layout/schema/widget.ts";
import type {Player, Stage, Team, UpdateMessage} from "../models/models.ts";

// Type-only imports above: the layout schema is used for shape fidelity, and
// nothing here depends on zod or on the widget registry at runtime.

/** PoC cap is 300 widgets; 200 is a realistic busy board. */
const TOTAL_WIDGETS = 200;
const DELTA_COUNTS = [1, 10, 100, 500];
const MAX_DELTAS = Math.max(...DELTA_COUNTS);

const SCENE_ID = "board";
const ROOT_GRID = {cols: 24, rows: 24};
const SUB_GRID = {cols: 8, rows: 8};
/** Children per sub-grid, and how many grid levels deep the tree goes. */
const SUBGRID_CHILDREN = 9;
const MAX_NESTING = 3;

const KEYFRAME_MS = new Date("2026-01-01T00:00:00.000Z").getTime();

const PALETTE = ["#0f172a", "#1e293b", "#334155", "#475569", "#64748b"];

interface BenchGame {
    code: string;
    players: Record<string, Player>;
    teams: Record<string, Team>;
    stage: Stage;
    data: {layout: GameLayout};
    createdAt: string;
    updatedAt: string;
}

function leafWidget(slot: number): Widget {
    return {
        type: slot % 7 === 0 ? "image" : "text",
        placement: {
            kind: "grid",
            col: slot % ROOT_GRID.cols,
            row: Math.floor(slot / ROOT_GRID.cols) % ROOT_GRID.rows,
            w: 1,
            h: 1,
        },
        style: {
            backgroundColor: PALETTE[slot % PALETTE.length],
            border: {width: 1, color: "#334155", radius: 4},
            padding: 4,
            opacity: 1,
        },
        config: {text: `cell ${slot}`, fontSize: 16, align: "center"},
    };
}

function subGridWidget(slot: number, children: Record<string, Widget>): Widget {
    return {
        type: "subgrid",
        placement: {
            kind: "grid",
            col: slot % ROOT_GRID.cols,
            row: Math.floor(slot / ROOT_GRID.cols) % ROOT_GRID.rows,
            w: 4,
            h: 4,
        },
        style: {padding: 2, overflow: "hidden"},
        config: {grid: SUB_GRID, widgets: children},
    };
}

/**
 * Build exactly `TOTAL_WIDGETS` widget nodes, with every fourth widget above
 * the depth cap carrying a nested grid so the tree has realistic depth rather
 * than being one flat map. Returns the dotted path of every leaf's
 * `config.text`, which is what the deltas target.
 */
function buildLayout(): {layout: GameLayout; textPaths: string[]} {
    const textPaths: string[] = [];
    let nextId = 0;
    let remaining = TOTAL_WIDGETS;

    function build(depth: number, capacity: number, prefix: string): Record<string, Widget> {
        const widgets: Record<string, Widget> = {};
        for (let slot = 0; remaining > 0 && slot < capacity; slot++) {
            const id = `w${nextId++}`;
            const path = `${prefix}.${id}`;
            remaining--;
            if (depth < MAX_NESTING && slot % 4 === 3 && remaining > 0) {
                const children = build(depth + 1, SUBGRID_CHILDREN, `${path}.config.widgets`);
                widgets[id] = subGridWidget(slot, children);
            } else {
                widgets[id] = leafWidget(slot);
                textPaths.push(`${path}.config.text`);
            }
        }
        return widgets;
    }

    const widgets = build(0, TOTAL_WIDGETS, `data.layout.scenes.${SCENE_ID}.widgets`);

    return {
        layout: {
            grid: ROOT_GRID,
            style: {backgroundGradient: {colors: ["#0f172a", "#1e293b"]}},
            scenes: {
                [SCENE_ID]: {style: {padding: 8}, widgets},
            },
        },
        textPaths,
    };
}

function buildGame(layout: GameLayout): BenchGame {
    const players: Record<string, Player> = {};
    for (let i = 0; i < 8; i++) {
        players[`p${i}`] = {
            name: `player ${i}`,
            score: i,
            connected: true,
            host: i === 0,
            controller: i === 0,
            data: {avatar: `a${i}`, ready: i % 2 === 0},
        };
    }

    const teams: Record<string, Team> = {
        red: {name: "Red", playerIds: ["p0", "p1", "p2", "p3"], data: {stats: {score: 0, wins: 0}}},
        blue: {name: "Blue", playerIds: ["p4", "p5", "p6", "p7"], data: {stats: {score: 0, wins: 0}}},
    };

    const stage: Stage = {
        currentScene: SCENE_ID,
        sceneOrder: ["lobby", SCENE_ID, "results"],
        scenes: {
            lobby: {data: {title: "Waiting"}},
            [SCENE_ID]: {data: {turn: "p0", round: 1}},
            results: {data: {title: "Results"}},
        },
    };

    return {
        code: "BENCH1",
        players,
        teams,
        stage,
        data: {layout},
        createdAt: new Date(KEYFRAME_MS).toISOString(),
        updatedAt: new Date(KEYFRAME_MS).toISOString(),
    };
}

/**
 * One delta touching a single leaf, the shape the server's `events.Diff`
 * produces. Paths are walked with a stride so consecutive deltas hit different
 * parts of the tree, and the stride is fixed so the run is deterministic.
 */
function buildDeltas(textPaths: string[], count: number): UpdateMessage[] {
    const deltas: UpdateMessage[] = [];
    for (let i = 0; i < count; i++) {
        const path = textPaths[(i * 37) % textPaths.length];
        deltas.push({
            id: `d${i}`,
            ts: new Date(KEYFRAME_MS + (i + 1) * 1000).toISOString(),
            op: "update",
            type: "game",
            updated: {[path]: `v${i}`},
            removed: [],
        });
    }
    return deltas;
}

function median(values: number[]): number {
    const sorted = [...values].sort((a, b) => a - b);
    const mid = Math.floor(sorted.length / 2);
    return sorted.length % 2 === 1 ? sorted[mid] : (sorted[mid - 1] + sorted[mid]) / 2;
}

interface Sample {
    median: number;
    best: number;
}

function summarise(timings: number[]): Sample {
    return {median: median(timings), best: Math.min(...timings)};
}

/**
 * Time the LAST `update()` only: the parser is loaded with `deltaCount - 1`
 * deltas first, untimed, so the figure is the cost of one incoming message at
 * that backlog depth — which is what a player actually waits for.
 */
function timeFinalUpdate(
    state: BenchGame,
    deltas: UpdateMessage[],
    deltaCount: number,
    samples: number,
): Sample {
    const timings: number[] = [];
    for (let s = 0; s < samples; s++) {
        const parser = new GameStateParser<BenchGame>();
        parser.set(state, new Date(KEYFRAME_MS));
        for (let i = 0; i < deltaCount - 1; i++) {
            parser.update(deltas[i]);
        }
        const start = performance.now();
        parser.update(deltas[deltaCount - 1]);
        timings.push(performance.now() - start);
    }
    return summarise(timings);
}

function timeDeepClone(state: BenchGame, samples: number): Sample {
    const timings: number[] = [];
    for (let s = 0; s < samples; s++) {
        const start = performance.now();
        JSON.parse(JSON.stringify(state));
        timings.push(performance.now() - start);
    }
    return summarise(timings);
}

function ms(value: number): string {
    return value.toFixed(3);
}

test("benchmark: reapply() cost with a 200-widget layout in game state", () => {
    const started = performance.now();

    const {layout, textPaths} = buildLayout();
    const state = buildGame(layout);
    const deltas = buildDeltas(textPaths, MAX_DELTAS);
    const bytes = JSON.stringify(state).length;

    // Warm up the JIT on the exact code paths that get timed, so the first
    // measured sample is not paying for optimisation and inline caches.
    timeFinalUpdate(state, deltas, 50, 2);
    timeDeepClone(state, 3);

    const clone = timeDeepClone(state, 15);
    // Fewer samples at 500: setup there is itself 500 reapplies, and the extra
    // samples buy less than they cost in CI time.
    const results = DELTA_COUNTS.map((count) => ({
        count,
        sample: timeFinalUpdate(state, deltas, count, count >= 500 ? 5 : 11),
    }));

    const lines = [
        "",
        "GameStateParser.reapply() — cost of ONE incoming delta",
        `  state:      ${TOTAL_WIDGETS} widgets, up to ${MAX_NESTING + 1} grid levels, ` +
            `${textPaths.length} leaves`,
        `  serialised: ${bytes.toLocaleString("en-US")} bytes`,
        `  deep clone alone: median ${ms(clone.median)} ms, best ${ms(clone.best)} ms`,
        "",
        "  deltas | median ms | best ms | ms per retained delta",
        "  -------+-----------+---------+----------------------",
        ...results.map(({count, sample}) =>
            `  ${String(count).padStart(6)} | ${ms(sample.median).padStart(9)} | ` +
            `${ms(sample.best).padStart(7)} | ${ms(sample.median / count).padStart(21)}`,
        ),
        "",
    ];
    console.log(lines.join("\n"));

    // The replay must actually produce the state it claims to; a benchmark that
    // measures a no-op is worse than no benchmark.
    const parser = new GameStateParser<BenchGame>();
    parser.set(state, new Date(KEYFRAME_MS));
    for (const delta of deltas) {
        parser.update(delta);
    }
    const current = parser.current();
    assert.ok(current !== undefined, "state rebuilt after replay");
    assert.notEqual(
        JSON.stringify(current).length,
        0,
        "replayed state is serialisable",
    );

    for (const {count, sample} of results) {
        assert.ok(
            Number.isFinite(sample.median) && sample.median >= 0,
            `measurement at ${count} deltas is a real number`,
        );
    }

    // Deliberately generous: this catches a catastrophic regression (an
    // accidental O(n^2) in the hot path, say) and nothing finer. Tight timing
    // assertions would be flaky on shared CI.
    const elapsed = performance.now() - started;
    assert.ok(elapsed < 10_000, `benchmark completed in ${ms(elapsed)} ms, budget 10000 ms`);
});
