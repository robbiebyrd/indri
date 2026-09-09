import test from "node:test";
import assert from "node:assert/strict";

import {StyleSchema} from "../schema/style.ts";
import {compileStyle} from "./compile.ts";

import type {Style} from "../schema/style.ts";

/**
 * Compile only what the schema accepted. Hand-built literals would let a test
 * pass on data the parser would have rejected, and would also carry explicit
 * `undefined` keys that `deepStrictEqual` counts as real.
 */
function parsed(raw: unknown): Style {
    const r = StyleSchema.safeParse(raw);
    assert.ok(r.success, `expected a valid style, got: ${r.success ? "" : r.error.message}`);
    return r.data;
}

function rejects(raw: unknown, why: string): void {
    assert.equal(StyleSchema.safeParse(raw).success, false, why);
}

test("a full style compiles to view style plus ordered layers", () => {
    const {viewStyle, layers} = compileStyle(parsed({
        backgroundColor: "#0f172a",
        backgroundImage: {uri: "https://example.test/bg.png", resizeMode: "contain"},
        backgroundGradient: {
            colors: ["#0f172a", "#1e293b"],
            locations: [0, 1],
            start: {x: 0, y: 0},
            end: {x: 1, y: 1},
        },
        border: {width: 2, color: "#334155", style: "dashed", radius: 8},
        padding: 8,
        opacity: 0.5,
        boxShadow: "0px 2px 4px rgba(0,0,0,0.4)",
        overflow: "hidden",
    }));

    assert.deepStrictEqual(viewStyle, {
        backgroundColor: "#0f172a",
        opacity: 0.5,
        overflow: "hidden",
        boxShadow: "0px 2px 4px rgba(0,0,0,0.4)",
        borderWidth: 2,
        borderColor: "#334155",
        borderStyle: "dashed",
        borderTopLeftRadius: 8,
        borderTopRightRadius: 8,
        borderBottomRightRadius: 8,
        borderBottomLeftRadius: 8,
        paddingTop: 8,
        paddingRight: 8,
        paddingBottom: 8,
        paddingLeft: 8,
    });

    assert.deepStrictEqual(layers, [
        {
            kind: "gradient",
            colors: ["#0f172a", "#1e293b"],
            locations: [0, 1],
            start: {x: 0, y: 0},
            end: {x: 1, y: 1},
        },
        {kind: "image", uri: "https://example.test/bg.png", resizeMode: "contain"},
    ]);
});

test("layer order is gradient then image, so the image paints on top", () => {
    const {layers} = compileStyle(parsed({
        backgroundImage: {uri: "https://example.test/bg.png"},
        backgroundGradient: {colors: ["#000", "#fff"]},
    }));

    assert.deepStrictEqual(layers.map(l => l.kind), ["gradient", "image"]);
});

test("a background image without a resize mode defaults to cover", () => {
    const {layers} = compileStyle(parsed({backgroundImage: {uri: "https://example.test/bg.png"}}));

    assert.deepStrictEqual(layers, [
        {kind: "image", uri: "https://example.test/bg.png", resizeMode: "cover"},
    ]);
});

test("an unknown style key is rejected rather than ignored", () => {
    rejects({backgroundColour: "#fff"}, "a misspelled top-level key must not be dropped silently");
    rejects({border: {width: 1, color: "#fff", radius: 4, glow: true}}, "nested keys are strict too");
});

test("boxShadow must carry a unit, because RN 0.79 rejects unitless lengths", () => {
    rejects({boxShadow: "1 1 black"}, "unitless lengths paint nothing on device");

    for (const shadow of ["1px 1px black", "0 0 2em #000", "0px 0px 1rem #000"]) {
        assert.equal(compileStyle(parsed({boxShadow: shadow})).viewStyle.boxShadow, shadow);
    }
});

test("radius expands to the four corner properties", () => {
    const perCorner = compileStyle(parsed({
        border: {width: 1, color: "#fff", radius: {topLeft: 1, topRight: 2, bottomRight: 3, bottomLeft: 4}},
    })).viewStyle;
    assert.deepStrictEqual(perCorner, {
        borderWidth: 1,
        borderColor: "#fff",
        borderTopLeftRadius: 1,
        borderTopRightRadius: 2,
        borderBottomRightRadius: 3,
        borderBottomLeftRadius: 4,
    });

    const scalar = compileStyle(parsed({border: {width: 1, color: "#fff", radius: 6}})).viewStyle;
    assert.deepStrictEqual(scalar, {
        borderWidth: 1,
        borderColor: "#fff",
        borderTopLeftRadius: 6,
        borderTopRightRadius: 6,
        borderBottomRightRadius: 6,
        borderBottomLeftRadius: 6,
    });

    const partial = compileStyle(parsed({border: {width: 1, color: "#fff", radius: {topLeft: 5}}})).viewStyle;
    assert.deepStrictEqual(partial, {borderWidth: 1, borderColor: "#fff", borderTopLeftRadius: 5});
});

test("padding expands from both the scalar and the per-edge form", () => {
    assert.deepStrictEqual(compileStyle(parsed({padding: 12})).viewStyle, {
        paddingTop: 12,
        paddingRight: 12,
        paddingBottom: 12,
        paddingLeft: 12,
    });

    assert.deepStrictEqual(compileStyle(parsed({padding: {top: 1, left: 2}})).viewStyle, {
        paddingTop: 1,
        paddingLeft: 2,
    });

    rejects({padding: {top: 1, start: 2}}, "per-edge padding uses left/right, not start/end");
});

test("border width, colour and style map straight through", () => {
    assert.deepStrictEqual(compileStyle(parsed({border: {width: 0, color: "#334155", style: "dotted"}})).viewStyle, {
        borderWidth: 0,
        borderColor: "#334155",
        borderStyle: "dotted",
    });
});

test("an absent style compiles to nothing instead of throwing", () => {
    assert.deepStrictEqual(compileStyle(undefined), {viewStyle: {}, layers: []});
    assert.deepStrictEqual(compileStyle(parsed({})), {viewStyle: {}, layers: []});
});

test("no per-platform shadow property is ever emitted", () => {
    // boxShadow is the single cross-platform shadow. Emitting iOS shadow* or
    // Android elevation alongside it would double-draw on one platform.
    const banned = ["shadowColor", "shadowOffset", "shadowOpacity", "shadowRadius", "elevation"];

    for (const raw of [
        {},
        {boxShadow: "0px 2px 4px #000"},
        {border: {width: 1, color: "#fff", radius: 4}, padding: 2, boxShadow: "1px 1px 1px #000"},
    ]) {
        const keys = Object.keys(compileStyle(parsed(raw)).viewStyle);
        for (const key of banned) assert.ok(!keys.includes(key), `${key} must never be emitted`);
    }
});

test("opacity outside 0..1 is rejected", () => {
    rejects({opacity: -0.1}, "opacity below 0 is not renderable");
    rejects({opacity: 1.1}, "opacity above 1 is not renderable");
    assert.equal(compileStyle(parsed({opacity: 0})).viewStyle.opacity, 0);
    assert.equal(compileStyle(parsed({opacity: 1})).viewStyle.opacity, 1);
});

test("a gradient needs at least two colours", () => {
    rejects({backgroundGradient: {colors: []}}, "an empty gradient has nothing to interpolate");
    rejects({backgroundGradient: {colors: ["#000"]}}, "one colour is a background colour, not a gradient");
});

test("overflow and background colour map straight through", () => {
    assert.deepStrictEqual(compileStyle(parsed({backgroundColor: "#123456", overflow: "visible"})).viewStyle, {
        backgroundColor: "#123456",
        overflow: "visible",
    });
    rejects({overflow: "scroll"}, "the vocabulary is closed: only visible and hidden are offered");
});

test("negative padding and radius are rejected", () => {
    // RN's behaviour for a negative padding or corner radius is platform
    // dependent, so these must not reach the compiler at all.
    assert.equal(StyleSchema.safeParse({padding: -1}).success, false);
    assert.equal(StyleSchema.safeParse({padding: {top: -1}}).success, false);
    assert.equal(StyleSchema.safeParse({border: {width: 1, color: "#000", radius: -1}}).success, false);
    assert.equal(
        StyleSchema.safeParse({border: {width: 1, color: "#000", radius: {topLeft: -1}}}).success,
        false,
    );
});
