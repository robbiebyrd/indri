/**
 * The shipped tic-tac-toe layout, exercised as data.
 *
 * These tests load the REAL `config.json` from the repository root — not a
 * fixture copy — because the thing under test is what actually ships. A copy
 * would go stale the first time someone edited the board and would then report
 * that a layout nobody runs is fine.
 *
 * The scene script is run for real, in the real Lua runtime, through the real
 * bridge. Cell ids are built with `string.format` at runtime, so nothing static
 * can tell you which widgets a script addresses or whether its 0-indexed ids
 * line up with Lua's 1-indexed tables. Running it is the only honest check.
 */

import assert from "node:assert/strict";
import {readFileSync} from "node:fs";
import {dirname, join} from "node:path";
import test from "node:test";
import {fileURLToPath} from "node:url";

import {LuaBridge} from "../lua/bridge.ts";
import {mergeOverrides} from "../lua/overrides.ts";
import {parseLayout} from "../schema/layout.ts";
import {imageDefinition} from "../../components/board/widgets/image/definition.ts";
import {subGridDefinition} from "../../components/board/widgets/subgrid/definition.ts";
import {textDefinition} from "../../components/board/widgets/text/definition.ts";

import type {StateKind, StateSource} from "../lua/bridge.ts";
import type {GameLayout} from "../schema/layout.ts";
import type {Widget} from "../schema/widget.ts";
import type {Game} from "../../models/models.ts";

// Resolved from this file, not from the working directory: the test runner is
// invoked from `client/`, but nothing about these tests should depend on that.
const REPO_ROOT = join(dirname(fileURLToPath(import.meta.url)), "..", "..", "..");

const SCENE_ID = "board";

/**
 * Exactly the types `components/board/widgets/index.ts` registers, taken from
 * the definitions rather than written out, so a renamed type fails here instead
 * of turning into an "unknown widget type" warning nobody reads.
 */
const KNOWN_WIDGET_TYPES: ReadonlySet<string> = new Set([
    textDefinition.type,
    imageDefinition.type,
    subGridDefinition.type,
]);

interface ShippedConfig {
    stage: {currentScene: string; scenes: Record<string, unknown>};
    data: {layout: unknown};
}

function loadConfig(relative: string): ShippedConfig {
    return JSON.parse(readFileSync(join(REPO_ROOT, relative), "utf8")) as ShippedConfig;
}

const CONFIG_PATHS = ["config.json", "example/tictactoe/config.json"];

/** The parsed shipped layout. Every test below builds on this one. */
function shippedLayout(): GameLayout {
    const config = loadConfig(CONFIG_PATHS[0]);
    const {layout, issues} = parse(config);

    assert.equal(issues.length, 0, describeIssues(issues));
    assert.ok(layout !== undefined);

    return layout;
}

function parse(config: ShippedConfig) {
    return parseLayout(config.data.layout, {
        knownWidgetTypes: KNOWN_WIDGET_TYPES,
        sceneIds: new Set(Object.keys(config.stage.scenes)),
    });
}

function describeIssues(issues: {severity: string; path: string; message: string}[]): string {
    return issues.map((i) => `${i.severity} ${i.path || "<root>"}: ${i.message}`).join("\n");
}

// ---- 1. the shipped layout is valid ---------------------------------------

// Zero ISSUES, not zero errors. A warning means the board still renders, with
// something quietly corrected — a clamped rect, an overlap, a scene with no
// matching stage entry — and a shipped layout has no excuse for any of them.
for (const path of CONFIG_PATHS) {
    test(`${path} carries a layout that parses with zero issues`, () => {
        const config = loadConfig(path);
        const {layout, issues} = parse(config);

        assert.equal(issues.length, 0, describeIssues(issues));
        assert.ok(layout !== undefined, "a layout that produced no issues must have parsed");
        assert.ok(
            layout.scenes[config.stage.currentScene] !== undefined,
            "the scene the stage opens on must exist in the layout",
        );
    });
}

test("both shipped configs carry the same layout", () => {
    const [root, example] = CONFIG_PATHS.map((p) => loadConfig(p).data.layout);

    // Two copies that drift are worse than one: the example would keep playing
    // while the default silently stopped, and the difference lives in a 60-line
    // JSON blob nobody diffs by eye.
    assert.deepEqual(example, root);
});

// ---- 2. the nine cells tile the sub-grid ----------------------------------

test("the 9 cells tile a 3x3 arrangement with no overlap and no gap", () => {
    const cells = cellWidgets(shippedLayout());
    const grid = subGrid(shippedLayout());

    assert.equal(Object.keys(cells).length, 9);

    const rects = Object.entries(cells).map(([id, widget]) => {
        assert.equal(widget.placement.kind, "grid", `${id} must be grid-placed to tile`);
        if (widget.placement.kind !== "grid") throw new Error("unreachable");

        return {id, ...widget.placement};
    });

    // Three distinct origins on each axis: a 3x3 ARRANGEMENT, whatever the
    // sub-grid's own resolution happens to be.
    assert.equal(new Set(rects.map((r) => r.row)).size, 3);
    assert.equal(new Set(rects.map((r) => r.col)).size, 3);

    // Every cell of the sub-grid is covered exactly once. This catches an
    // overlap and a gap with the same count, which a pairwise collision check
    // would not: nine rects can miss a column without ever touching.
    const cover = new Map<string, string[]>();
    for (const rect of rects) {
        for (let row = rect.row; row < rect.row + rect.h; row++) {
            for (let col = rect.col; col < rect.col + rect.w; col++) {
                const key = `${row},${col}`;
                cover.set(key, [...(cover.get(key) ?? []), rect.id]);
            }
        }
    }

    const overlapping = [...cover].filter(([, ids]) => ids.length > 1);
    assert.deepEqual(overlapping, [], "no sub-grid cell may be claimed by two widgets");
    assert.equal(
        cover.size,
        grid.cols * grid.rows,
        `the 9 cells must cover all ${grid.cols}x${grid.rows} of the sub-grid`,
    );
});

// ---- 3./4. the script, run for real ---------------------------------------

/** A board whose transpose differs from itself in every interesting way. */
const BOARD = [
    ["X", "", ""],
    ["O", "X", ""],
    ["", "", "O"],
];

const EXPECTED_TEXT: Record<string, string> = {
    c00: "X", c01: "", c02: "",
    c10: "O", c11: "X", c12: "",
    c20: "", c21: "", c22: "O",
};

test("the scene script paints board state onto the cells without transposing it", () => {
    const h = harness();
    h.push(gameWith(BOARD));

    assert.deepEqual(h.errors, [], "the shipped script must run clean");

    const painted = Object.fromEntries(
        Object.keys(EXPECTED_TEXT).map((id) => [id, h.cellText(id)]),
    );

    // Asserted as one object rather than cell by cell: a transposed script
    // fails on exactly two cells here, and seeing the whole board is what makes
    // "row and column are swapped" obvious instead of "c10 was wrong".
    assert.deepEqual(painted, EXPECTED_TEXT);

    h.dispose();
});

test("a later board reaches the cells too, so the binding is not a one-shot", () => {
    const h = harness();
    h.push(gameWith(BOARD));

    const next = BOARD.map((row) => [...row]);
    next[0][2] = "O";
    // A delta, not a keyframe: a keyframe clears the override layer, so this
    // also proves the repaint is the script's doing and not a leftover.
    h.push(gameWith(next), "delta");

    assert.equal(h.cellText("c02"), "O");
    assert.equal(h.cellText("c00"), "X", "the untouched cells keep their text");
    assert.deepEqual(h.errors, []);
    h.dispose();
});

test("every widget id the script writes to exists in the layout", () => {
    const h = harness();
    h.push(gameWith(BOARD));

    const layout = shippedLayout();
    const known = new Set(Object.keys(allWidgets(layout)));
    const written = Object.keys(h.bridge.host.overrides.snapshot().widgets[SCENE_ID] ?? {});

    // Ids are built with `string.format` at runtime, so this is the only way to
    // learn which widgets the script actually addresses. A write to an id the
    // layout does not contain is silent — the override simply never merges onto
    // anything — which is exactly the failure this test exists to make loud.
    assert.equal(written.length, 9, "the script addresses the nine cells");
    for (const id of written) {
        assert.ok(known.has(id), `script wrote to "${id}", which is not in the layout`);
    }

    h.dispose();
});

// ---- 5. the press path ----------------------------------------------------

test("pressing a cell sends the move the Go handler expects, row first", () => {
    const h = harness();
    h.push(gameWith(BOARD));

    h.press("c12");

    assert.deepEqual(
        h.sent,
        [{action: "move", move: "1,2"}],
        "c12 is row 1, column 2 — the same order the Go handler splits on",
    );
    assert.deepEqual(h.errors, []);
    h.dispose();
});

test("pressing a widget that is not a cell sends nothing", () => {
    const h = harness();
    h.push(gameWith(BOARD));

    // The scene handler sees every widget in its scene. An unguarded script
    // would turn this into `move = "i,t"` and make the server reject a message
    // the player never meant to send.
    h.press("title");

    assert.deepEqual(h.sent, []);
    assert.deepEqual(h.errors, []);
    h.dispose();
});

// ---- harness --------------------------------------------------------------

interface Harness {
    readonly bridge: LuaBridge;
    readonly sent: Record<string, unknown>[];
    readonly errors: string[];
    push(game: Game, kind?: StateKind): void;
    /** The cell's text AFTER the override layer is composited over the layout. */
    cellText(id: string): unknown;
    press(widgetId: string): void;
    dispose(): void;
}

function harness(): Harness {
    const sent: Record<string, unknown>[] = [];
    const errors: string[] = [];
    const listeners = new Set<(game: Game, kind: StateKind) => void>();

    const state: StateSource = {
        observe(listener) {
            listeners.add(listener);

            return () => {
                listeners.delete(listener);
            };
        },
    };

    const bridge = new LuaBridge({
        socket: {send: (message) => sent.push(message as Record<string, unknown>)},
        state,
        onError: (e) => errors.push(e),
    });

    const layout = shippedLayout();

    return {
        bridge,
        sent,
        errors,
        push(game, kind = "keyframe") {
            for (const listener of listeners) listener(game, kind);
        },
        cellText(id) {
            const merged = mergeOverrides(layout, bridge.host.overrides.snapshot());

            return cellWidgets(merged)[id]?.config?.text;
        },
        press(widgetId) {
            bridge.host.emit(
                "widgetPress",
                {kind: "widget", sceneId: SCENE_ID, widgetId},
                widgetId,
            );
        },
        dispose: () => bridge.dispose(),
    };
}

/** The game shape the server actually broadcasts, with `board` swapped in. */
function gameWith(board: string[][]): Game {
    const config = loadConfig(CONFIG_PATHS[0]);

    return {
        code: "ABCD",
        stage: {
            currentScene: SCENE_ID,
            sceneOrder: [SCENE_ID],
            scenes: {[SCENE_ID]: {data: {board}}},
        },
        data: {layout: config.data.layout as Record<string, unknown>},
    } as Game;
}

// ---- layout navigation ----------------------------------------------------

function scene(layout: GameLayout) {
    const found = layout.scenes[SCENE_ID];
    assert.ok(found !== undefined, `the layout has no "${SCENE_ID}" scene`);

    return found;
}

function cellsWidget(layout: GameLayout): Widget {
    const cells = scene(layout).widgets.cells;
    assert.ok(cells !== undefined, "the layout has no \"cells\" sub-grid");

    return cells;
}

function subGrid(layout: GameLayout): {cols: number; rows: number} {
    const parsed = subGridDefinition.schema.safeParse(cellsWidget(layout).config ?? {});
    assert.ok(parsed.success, "the \"cells\" widget must hold a valid sub-grid config");

    return parsed.data.grid;
}

function cellWidgets(layout: GameLayout): Record<string, Widget> {
    const parsed = subGridDefinition.schema.safeParse(cellsWidget(layout).config ?? {});
    assert.ok(parsed.success, "the \"cells\" widget must hold a valid sub-grid config");

    return parsed.data.widgets;
}

/** Every widget in the scene, at every depth — ids are unique within a scene. */
function allWidgets(layout: GameLayout): Record<string, Widget> {
    return {...scene(layout).widgets, ...cellWidgets(layout)};
}
