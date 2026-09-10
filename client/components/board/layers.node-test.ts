import assert from "node:assert/strict";
import {test} from "node:test";

import {contentFitFor, gradientColors, gradientLocations} from "./layers.ts";

test("a colour list becomes the tuple the gradient component requires", () => {
    assert.deepEqual(gradientColors(["#000", "#fff"]), ["#000", "#fff"]);
    assert.deepEqual(gradientColors(["#000", "#888", "#fff"]), ["#000", "#888", "#fff"]);
});

// The native gradient view crashes on a single stop. `StyleSchema` already
// demands two, so reaching this means the style was built in code — the layer
// is skipped rather than taking the app down with it.
test("fewer than two colours yields no gradient at all", () => {
    assert.equal(gradientColors([]), undefined);
    assert.equal(gradientColors(["#000"]), undefined);
});

test("colour-stop positions survive when they match the colours", () => {
    assert.deepEqual(gradientLocations([0, 1], 2), [0, 1]);
    assert.deepEqual(gradientLocations([0, 0.8, 1], 3), [0, 0.8, 1]);
});

// A length mismatch is undefined behaviour in the native view: it throws on
// some platforms and renders the wrong stops on others. An evenly distributed
// gradient is a better failure than either, so the positions are dropped and
// the layer is kept.
test("mismatched colour-stop positions are dropped, not passed through", () => {
    assert.equal(gradientLocations([0, 0.5, 1], 2), undefined);
    assert.equal(gradientLocations([0, 1], 3), undefined);
    assert.equal(gradientLocations([0.5], 1), undefined);
    assert.equal(gradientLocations(undefined, 2), undefined);
});

test("every resize mode maps onto a contentFit expo-image accepts", () => {
    assert.equal(contentFitFor("cover"), "cover");
    assert.equal(contentFitFor("contain"), "contain");
    assert.equal(contentFitFor("stretch"), "fill");
    assert.equal(contentFitFor("center"), "none");
});

// expo-image cannot tile, and `contentFit` is modelled on CSS `object-fit`,
// which has no tiling value either. `none` keeps the image at the natural size
// a tile would have used. Pinned so the degradation is a decision, not a bug.
test("repeat degrades to none, because expo-image cannot tile", () => {
    assert.equal(contentFitFor("repeat"), "none");
});
