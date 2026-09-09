import test from "node:test";
import assert from "node:assert/strict";

import {GameLayoutSchema, parseLayout} from "./layout.ts";
import {Placement} from "./placement.ts";
import {MAX_SUBGRID_DEPTH} from "./widget.ts";

import type {LayoutIssue} from "./layout.ts";

function textWidget(col: number, row: number, w = 1, h = 1): unknown {
    return {type: "text", placement: {kind: "grid", col, row, w, h}, config: {text: ""}};
}

function layoutWith(widgets: Record<string, unknown>, grid = {cols: 12, rows: 12}): unknown {
    return {grid, scenes: {board: {widgets}}};
}

function messages(issues: LayoutIssue[], severity?: LayoutIssue["severity"]): string {
    return issues.filter((i) => !severity || i.severity === severity).map((i) => `${i.path}: ${i.message}`).join(" | ");
}

function warnings(issues: LayoutIssue[]): LayoutIssue[] {
    return issues.filter((i) => i.severity === "warning");
}

function errors(issues: LayoutIssue[]): LayoutIssue[] {
    return issues.filter((i) => i.severity === "error");
}

// --- happy path -------------------------------------------------------------

test("a well-formed layout parses with zero issues", () => {
    const raw = {
        grid: {cols: 12, rows: 12},
        style: {backgroundColor: "#0f172a"},
        script: "-- game script",
        scenes: {
            board: {
                style: {padding: 8},
                script: "-- scene script",
                widgets: {
                    title: {
                        type: "text",
                        placement: {kind: "grid", col: 0, row: 0, w: 12, h: 2},
                        style: {border: {width: 2, color: "#334155", radius: 8}},
                        config: {text: "Tic Tac Toe"},
                    },
                    badge: {
                        type: "image",
                        placement: {
                            kind: "absolute",
                            left: "80%", top: "4%", width: "16%", height: "16%", z: 10,
                        },
                        config: {uri: "https://example/turn.png"},
                    },
                },
            },
        },
    };
    const {layout, issues} = parseLayout(raw);
    assert.equal(issues.length, 0, messages(issues));
    assert.ok(layout);
    assert.equal(layout.grid.cols, 12);
    assert.equal(Object.keys(layout.scenes.board.widgets).length, 2);
});

test("widgets are a map keyed by id, never an array", () => {
    // events.Diff replaces arrays whole, so an array here would re-send every
    // widget on any single change and defeat the delta pipeline.
    const asArray = {grid: {cols: 8, rows: 8}, scenes: {board: {widgets: [textWidget(0, 0)]}}};
    assert.equal(parseLayout(asArray).layout, undefined);
    assert.ok(errors(parseLayout(asArray).issues).length > 0);
});

test("parseLayout never throws, whatever it is handed", () => {
    for (const bad of [undefined, null, 0, "", [], {}, {grid: null}, {scenes: 5}, NaN]) {
        assert.doesNotThrow(() => parseLayout(bad), `threw on ${JSON.stringify(bad) ?? "undefined"}`);
        assert.equal(parseLayout(bad).layout, undefined);
    }
});

// --- rejected outright (error, no layout) -----------------------------------

test("a privateData key anywhere is a fatal error", () => {
    const nested = {
        grid: {cols: 8, rows: 8},
        scenes: {board: {widgets: {a: {
            type: "text",
            placement: {kind: "grid", col: 0, row: 0, w: 1, h: 1},
            config: {deep: {privateData: {secret: 1}}},
        }}}},
    };
    const {layout, issues} = parseLayout(nested);
    assert.equal(layout, undefined, "must not render a layout containing a reserved key");
    assert.equal(errors(issues).length, 1);
    // The reason matters more than the rejection: SanitizeDelta strips any path
    // containing a privateData segment at ANY depth, so the value would vanish
    // in transit with nothing to explain the difference.
    assert.match(issues[0].message, /SanitizeDelta/);
    assert.match(issues[0].path, /config\.deep\.privateData/);
});

test("a pixel value in absolute placement is a fatal error", () => {
    const pixels = layoutWith({
        a: {type: "text", placement: {kind: "absolute", left: 40, top: 10, width: 20, height: 20}},
    });
    const {layout, issues} = parseLayout(pixels);
    assert.equal(layout, undefined);
    assert.ok(errors(issues).length > 0, messages(issues));
});

test("percent strings are the only representable absolute placement", () => {
    assert.equal(
        Placement.safeParse({kind: "absolute", left: "5%", top: "5%", width: "5%", height: "5%"}).success,
        true,
    );
    for (const bad of [5, "5", "5px", "5 %", "%"]) {
        assert.equal(
            Placement.safeParse({kind: "absolute", left: bad, top: "5%", width: "5%", height: "5%"}).success,
            false,
            `${JSON.stringify(bad)} must not parse as a percentage`,
        );
    }
});

test("grid dimensions outside 8..4096 are a fatal error", () => {
    for (const cols of [7, 4097, 0, -1, 12.5]) {
        const {layout} = parseLayout(layoutWith({}, {cols, rows: 12}));
        assert.equal(layout, undefined, `cols ${cols} must be rejected`);
    }
    assert.ok(parseLayout(layoutWith({}, {cols: 8, rows: 8})).layout);
    assert.ok(parseLayout(layoutWith({}, {cols: 4096, rows: 4096})).layout);
});

// --- tolerated (warning, layout still renders) ------------------------------

test("overlapping non-absolute widgets warn but still render", () => {
    const {layout, issues} = parseLayout(layoutWith({
        a: textWidget(0, 0, 3, 3),
        b: textWidget(1, 1, 3, 3),
    }));
    assert.ok(layout, "a board with an overlap is still better than no board");
    assert.equal(warnings(issues).length, 1, messages(issues));
    assert.match(issues[0].message, /overlaps "b"/);
});

test("absolute widgets may overlap freely and raise nothing", () => {
    const abs = (left: string) => ({
        type: "image",
        placement: {kind: "absolute", left, top: "0%", width: "50%", height: "50%"},
    });
    const {layout, issues} = parseLayout(layoutWith({a: abs("0%"), b: abs("10%")}));
    assert.ok(layout);
    assert.equal(issues.length, 0, messages(issues));
});

test("an out-of-bounds rect is clamped, warned about, and still rendered", () => {
    const {layout, issues} = parseLayout(layoutWith({a: textWidget(20, 20, 4, 4)}));
    assert.ok(layout);
    assert.equal(warnings(issues).length, 1, messages(issues));
    const p = layout.scenes.board.widgets.a.placement;
    assert.equal(p.kind, "grid");
    if (p.kind === "grid") {
        assert.ok(p.col + p.w <= 12 && p.row + p.h <= 12, "clamped inside the grid");
    }
});

test("zero-size and fractional rects are clamped rather than rejected", () => {
    for (const bad of [textWidget(0, 0, 0, 1), textWidget(0, 0, -2, 1), textWidget(0.5, 1.5, 1, 1)]) {
        const {layout, issues} = parseLayout(layoutWith({a: bad}));
        assert.ok(layout, "the rest of the board must still render");
        assert.equal(warnings(issues).length, 1, messages(issues));
        const p = layout.scenes.board.widgets.a.placement;
        if (p.kind === "grid") {
            assert.ok(Number.isInteger(p.col) && Number.isInteger(p.row), "normalised to integers");
            assert.ok(p.w > 0 && p.h > 0, "normalised to a positive span");
        }
    }
});

test("the returned layout is normalised, so renderers need not re-check geometry", () => {
    const {layout} = parseLayout(layoutWith({
        a: textWidget(-5, -5, 0, 0),
        b: textWidget(99, 99, 99, 99),
    }));
    assert.ok(layout);
    for (const w of Object.values(layout.scenes.board.widgets)) {
        if (w.placement.kind === "grid") {
            const {col, row, w: ww, h} = w.placement;
            assert.ok(Number.isInteger(col) && col >= 0 && Number.isInteger(row) && row >= 0);
            assert.ok(ww > 0 && h > 0 && col + ww <= 12 && row + h <= 12);
        }
    }
});

test("an unknown widget type warns only when the caller supplies the known set", () => {
    const raw = layoutWith({a: {...(textWidget(0, 0) as object), type: "hologram"}});
    assert.equal(parseLayout(raw).issues.length, 0, "no registry supplied, so no type check");

    const {layout, issues} = parseLayout(raw, {knownWidgetTypes: new Set(["text", "image"])});
    assert.ok(layout);
    assert.equal(warnings(issues).length, 1);
    assert.match(issues[0].message, /unknown widget type "hologram"/);
});

test("a layout scene with no matching stage scene warns", () => {
    const {layout, issues} = parseLayout(layoutWith({}), {sceneIds: new Set(["lobby"])});
    assert.ok(layout, "scenes and layouts are edited independently, so this is not fatal");
    assert.equal(warnings(issues).length, 1);
    assert.match(issues[0].message, /no matching entry in stage\.scenes/);
});

// --- sub-grid recursion -----------------------------------------------------

function nestSubGrids(depth: number): unknown {
    let inner: Record<string, unknown> = {leaf: textWidget(0, 0)};
    for (let i = 0; i < depth; i++) {
        inner = {
            sg: {
                type: "subgrid",
                placement: {kind: "grid", col: 0, row: 0, w: 4, h: 4},
                config: {grid: {cols: 8, rows: 8}, widgets: inner},
            },
        };
    }
    return inner;
}

test("sub-grids nest up to the depth cap and are normalised at every level", () => {
    const {layout, issues} = parseLayout(layoutWith(nestSubGrids(MAX_SUBGRID_DEPTH - 1) as Record<string, unknown>));
    assert.ok(layout);
    assert.equal(warnings(issues).length, 0, messages(issues));
});

test("sub-grid nesting past the cap warns and drops the excess children", () => {
    const {layout, issues} = parseLayout(layoutWith(nestSubGrids(MAX_SUBGRID_DEPTH + 2) as Record<string, unknown>));
    assert.ok(layout, "an over-deep payload must not blank the board");
    const capped = warnings(issues).filter((i) => /exceeds the maximum depth/.test(i.message));
    assert.ok(capped.length > 0, messages(issues));
});

test("a malformed sub-grid config warns without taking down the layout", () => {
    const {layout, issues} = parseLayout(layoutWith({
        sg: {type: "subgrid", placement: {kind: "grid", col: 0, row: 0, w: 2, h: 2}, config: {grid: "nope"}},
    }));
    assert.ok(layout);
    assert.equal(warnings(issues).length, 1, messages(issues));
    assert.match(issues[0].message, /sub-grid config is malformed/);
});

test("collision is scoped per grid level", () => {
    // Two children overlapping INSIDE a sub-grid is one warning; the sub-grid
    // itself sitting over a sibling on the parent grid is a separate one.
    const {issues} = parseLayout(layoutWith({
        sg: {
            type: "subgrid",
            placement: {kind: "grid", col: 0, row: 0, w: 4, h: 4},
            config: {grid: {cols: 8, rows: 8}, widgets: {x: textWidget(0, 0, 3, 3), y: textWidget(1, 1, 3, 3)}},
        },
    }));
    assert.equal(warnings(issues).length, 1, messages(issues));
    assert.match(issues[0].path, /sg\.config\.widgets\.x$/);
});

// --- schema surface ---------------------------------------------------------

test("unknown top-level and scene keys are rejected", () => {
    assert.equal(GameLayoutSchema.safeParse({grid: {cols: 8, rows: 8}, scenes: {}, colour: "red"}).success, false);
    assert.equal(
        GameLayoutSchema.safeParse({grid: {cols: 8, rows: 8}, scenes: {b: {widgets: {}, oops: 1}}}).success,
        false,
    );
});
