/**
 * The built-in widgets' contracts, exercised in bare Node.
 *
 * The three `definition.ts` modules are imported, never the `.tsx` components
 * beside them: `node --experimental-strip-types` cannot load JSX and
 * `react-native` is not loadable outside a bundler, so importing a component
 * would take the whole file out. That split is exactly why the definitions
 * live in their own modules — see `./index.ts`.
 */

import assert from "node:assert/strict";
import test, {beforeEach} from "node:test";

import {parseLayout} from "../../../layout/schema/layout.ts";
import {
    assertDescriptorsMatchSchema,
    clearRegistry,
    knownWidgetTypes,
    registerWidget,
} from "../../../layout/registry/registry.ts";
import {MAX_SUBGRID_DEPTH} from "../../../layout/schema/widget.ts";
import {imageDefinition} from "./image/definition.ts";
import {subGridDefinition} from "./subgrid/definition.ts";
import {textDefinition} from "./text/definition.ts";

import type {WidgetDefinition} from "../../../layout/registry/registry.ts";

// The renderer-free halves of exactly the definitions `./index.ts` registers.
const DEFINITIONS: WidgetDefinition[] = [
    textDefinition as WidgetDefinition,
    imageDefinition as WidgetDefinition,
    subGridDefinition as WidgetDefinition,
];

// The registry is module-global, so every test starts from empty.
beforeEach(clearRegistry);

// --- descriptors agree with schemas -------------------------------------------

// The bidirectional check is the entire reason descriptors are hand-written
// rather than derived from zod internals, so it has to run against the real
// widgets and not only against the registry's own fixture.
for (const def of DEFINITIONS) {
    test(`"${def.type}" descriptors and config schema describe the same keys`, () => {
        assert.doesNotThrow(() => assertDescriptorsMatchSchema(def));
    });
}

// --- defaults ------------------------------------------------------------------

for (const def of DEFINITIONS) {
    test(`"${def.type}" defaults parse against its own schema`, () => {
        const parsed = def.schema.safeParse(def.defaults);

        assert.equal(parsed.success, true, JSON.stringify(parsed.error?.issues));
    });
}

// --- registration and parseLayout ----------------------------------------------

function registerAll(): void {
    for (const def of DEFINITIONS) registerWidget(def);
}

test("registering the built-in set yields exactly text, image and subgrid", () => {
    registerAll();

    assert.deepEqual([...knownWidgetTypes()].sort(), ["image", "subgrid", "text"]);
});

test("a layout using all three types parses with zero issues", () => {
    registerAll();

    const raw = {
        grid: {cols: 12, rows: 12},
        scenes: {
            board: {
                widgets: {
                    title: {
                        type: "text",
                        placement: {kind: "grid", col: 0, row: 0, w: 12, h: 2},
                        config: {text: "Tic Tac Toe", fontSize: 32, align: "center"},
                    },
                    badge: {
                        type: "image",
                        placement: {
                            kind: "absolute",
                            left: "80%", top: "4%", width: "16%", height: "16%", z: 10,
                        },
                        config: {uri: "https://example.test/turn.png", resizeMode: "contain"},
                    },
                    cells: {
                        type: "subgrid",
                        placement: {kind: "grid", col: 3, row: 3, w: 6, h: 6},
                        config: {
                            grid: {cols: 9, rows: 9},
                            widgets: {
                                c00: {
                                    type: "text",
                                    placement: {kind: "grid", col: 0, row: 0, w: 3, h: 3},
                                    config: {text: ""},
                                },
                            },
                        },
                    },
                },
            },
        },
    };

    const {layout, issues} = parseLayout(raw, {knownWidgetTypes: knownWidgetTypes()});

    assert.deepEqual(issues, []);
    assert.ok(layout);
});

test("the same layout warns about every type once the registry is empty", () => {
    // Guards the test above from passing for the wrong reason: if the layout
    // were somehow type-checked against nothing, an empty registry would also
    // produce zero issues.
    const raw = {
        grid: {cols: 12, rows: 12},
        scenes: {
            board: {
                widgets: {
                    a: {type: "text", placement: {kind: "grid", col: 0, row: 0, w: 1, h: 1}},
                },
            },
        },
    };

    const {issues} = parseLayout(raw, {knownWidgetTypes: knownWidgetTypes()});

    assert.deepEqual(issues.map((i) => i.severity), ["warning"]);
});

// --- each schema rejects a representative bad value -----------------------------

test("text config rejects an unknown align", () => {
    const parsed = textDefinition.schema.safeParse({text: "hi", align: "middle"});

    assert.equal(parsed.success, false);
});

test("text config rejects a non-string text", () => {
    assert.equal(textDefinition.schema.safeParse({text: 42}).success, false);
});

test("image config rejects a non-string uri", () => {
    const parsed = imageDefinition.schema.safeParse({uri: 42});

    assert.equal(parsed.success, false);
});

test("image config accepts a data: URI and a relative path, which .url() would reject", () => {
    // The reason `uri` is `z.string()` and not `.url()`. A source that does
    // not resolve is a render concern, not a parse one.
    assert.equal(imageDefinition.schema.safeParse({uri: "data:image/gif;base64,R0lGOD"}).success, true);
    assert.equal(imageDefinition.schema.safeParse({uri: "./assets/turn.png"}).success, true);
});

test("sub-grid config rejects a grid below the minimum dimension", () => {
    const parsed = subGridDefinition.schema.safeParse({grid: {cols: 2, rows: 2}, widgets: {}});

    assert.equal(parsed.success, false);
});

test("sub-grid config rejects a widgets array, which the delta pipeline cannot use", () => {
    const parsed = subGridDefinition.schema.safeParse({grid: {cols: 8, rows: 8}, widgets: []});

    assert.equal(parsed.success, false);
});

// --- recursion ------------------------------------------------------------------

/** `depth` nested sub-grid widgets, with one text widget at the bottom. */
function nestedSubGrid(depth: number): Record<string, unknown> {
    let inner: Record<string, unknown> = {
        type: "text",
        placement: {kind: "grid", col: 0, row: 0, w: 1, h: 1},
        config: {text: "leaf"},
    };

    for (let i = 0; i < depth; i++) {
        inner = {
            type: "subgrid",
            placement: {kind: "grid", col: 0, row: 0, w: 8, h: 8},
            config: {grid: {cols: 8, rows: 8}, widgets: {child: inner}},
        };
    }

    return inner;
}

function sceneWith(widget: Record<string, unknown>): unknown {
    return {grid: {cols: 8, rows: 8}, scenes: {board: {widgets: {root: widget}}}};
}

test("the sub-grid schema accepts nesting to MAX_SUBGRID_DEPTH", () => {
    const nested = nestedSubGrid(MAX_SUBGRID_DEPTH) as {config: unknown};
    const parsed = subGridDefinition.schema.safeParse(nested.config);

    assert.equal(parsed.success, true, JSON.stringify(parsed.error?.issues));
});

test("parseLayout renders a sub-grid chain up to the depth cap without complaint", () => {
    registerAll();

    // MAX_SUBGRID_DEPTH grids in total: the scene's own, plus one per
    // sub-grid. The leaf text widget sits in the last of them.
    const {layout, issues} = parseLayout(sceneWith(nestedSubGrid(MAX_SUBGRID_DEPTH - 1)), {
        knownWidgetTypes: knownWidgetTypes(),
    });

    assert.deepEqual(issues, []);
    assert.ok(layout);
});

test("parseLayout drops the children of a sub-grid past the depth cap", () => {
    registerAll();

    const {layout, issues} = parseLayout(sceneWith(nestedSubGrid(MAX_SUBGRID_DEPTH)), {
        knownWidgetTypes: knownWidgetTypes(),
    });

    assert.deepEqual(issues.map((i) => i.severity), ["warning"]);
    assert.match(issues[0].message, /exceeds the maximum depth of 4/);
    // The board still renders — an over-deep sub-grid loses its children, not
    // the whole layout.
    assert.ok(layout);
});
