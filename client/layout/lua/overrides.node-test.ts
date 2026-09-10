import test from "node:test";
import assert from "node:assert/strict";

import {EMPTY_OVERRIDES, OverrideLayer, mergeOverrides} from "./overrides.ts";
import {SUBGRID_TYPE} from "../schema/widget.ts";

import type {GameLayout} from "../schema/layout.ts";
import type {Widget} from "../schema/widget.ts";

function cell(col: number, row: number): Widget {
    return {type: "text", placement: {kind: "grid", col, row, w: 1, h: 1}};
}

/**
 * A board with a styled title and a sub-grid holding two cells, so the tests
 * cover both a top-level widget and a nested one — a tic-tac-toe cell lives at
 * the nested depth, and that is the one a scene script actually paints.
 */
function layout(): GameLayout {
    return {
        grid: {cols: 12, rows: 12},
        style: {backgroundColor: "black", opacity: 1},
        scenes: {
            board: {
                style: {backgroundColor: "navy"},
                widgets: {
                    title: {
                        type: "text",
                        placement: {kind: "grid", col: 1, row: 1, w: 12, h: 2},
                        style: {backgroundColor: "white"},
                        config: {text: "hello", align: "center"},
                    },
                    grid: {
                        type: SUBGRID_TYPE,
                        placement: {kind: "grid", col: 1, row: 3, w: 6, h: 6},
                        config: {
                            grid: {cols: 8, rows: 8},
                            widgets: {"cell-0": cell(1, 1), "cell-1": cell(2, 1)},
                        },
                    },
                },
            },
            lobby: {widgets: {waiting: cell(1, 1)}},
        },
    };
}

/** Deep structural copy, for proving the input came back untouched. */
function frozenCopy(value: unknown): string {
    return JSON.stringify(value);
}

test("a fresh layer is empty and merging it returns the very layout it was given", () => {
    const layer = new OverrideLayer();
    assert.equal(layer.snapshot(), EMPTY_OVERRIDES);

    const server = layout();
    assert.equal(
        mergeOverrides(server, layer.snapshot()),
        server,
        "no overrides must mean no copying at all, so the renderer can memoise on identity",
    );
});

test("widget setStyle and setConfig write to the override map and nothing else", () => {
    const layer = new OverrideLayer();
    layer.setWidgetStyle("board", "title", {opacity: 0.5});
    layer.setWidgetConfig("board", "title", {text: "goodbye"});

    assert.deepEqual(layer.snapshot().widgets, {
        board: {title: {style: {opacity: 0.5}, config: {text: "goodbye"}}},
    });
    assert.deepEqual(layer.snapshot().board, {}, "a widget write must not touch the board scope");
    assert.deepEqual(layer.snapshot().scenes, {}, "…nor the scene scope");
});

test("writes accumulate key by key instead of replacing the whole patch", () => {
    const layer = new OverrideLayer();
    layer.setWidgetStyle("board", "title", {opacity: 0.5});
    layer.setWidgetStyle("board", "title", {backgroundColor: "red"});

    assert.deepEqual(
        layer.snapshot().widgets.board["title"].style,
        {opacity: 0.5, backgroundColor: "red"},
        "a second setStyle must not silently drop the first one's keys",
    );
});

test("widget ids are scoped per scene, so the same id in two scenes stays separate", () => {
    const layer = new OverrideLayer();
    layer.setWidgetStyle("board", "title", {opacity: 0.5});
    layer.setWidgetStyle("lobby", "title", {opacity: 0.1});

    assert.equal(layer.snapshot().widgets.board["title"].style?.opacity, 0.5);
    assert.equal(layer.snapshot().widgets.lobby["title"].style?.opacity, 0.1);
});

test("the snapshot identity changes only when something changed", () => {
    const layer = new OverrideLayer();
    const before = layer.snapshot();

    layer.clearForKeyframe();
    assert.equal(layer.snapshot(), before, "clearing an empty layer is not a change");

    layer.setBoardStyle({opacity: 0.5});
    assert.notEqual(layer.snapshot(), before);
});

test("subscribers are notified on every write and unsubscribe stops them", () => {
    const layer = new OverrideLayer();
    let notified = 0;
    const unsubscribe = layer.subscribe(() => {
        notified++;
    });

    layer.setBoardStyle({opacity: 0.5});
    layer.setSceneStyle("board", {opacity: 0.5});
    assert.equal(notified, 2);

    unsubscribe();
    layer.setBoardStyle({opacity: 0.25});
    assert.equal(notified, 2, "an unsubscribed listener must not be called");
});

test("mergeOverrides produces the effective layout without mutating the input", () => {
    const server = layout();
    const before = frozenCopy(server);

    const layer = new OverrideLayer();
    layer.setBoardStyle({backgroundColor: "purple"});
    layer.setSceneStyle("board", {opacity: 0.9});
    layer.setWidgetStyle("board", "title", {opacity: 0.5});
    layer.setWidgetConfig("board", "title", {text: "goodbye"});
    layer.setWidgetStyle("board", "cell-1", {backgroundColor: "gold"});

    const effective = mergeOverrides(server, layer.snapshot());

    // The board keeps `opacity: 1` from the author and takes the new colour.
    assert.deepEqual(effective.style, {backgroundColor: "purple", opacity: 1});
    assert.deepEqual(effective.scenes.board.style, {backgroundColor: "navy", opacity: 0.9});

    const title = effective.scenes.board.widgets.title;
    assert.deepEqual(title.style, {backgroundColor: "white", opacity: 0.5});
    assert.deepEqual(
        title.config,
        {text: "goodbye", align: "center"},
        "a config patch merges key by key; `align` was not named and must survive",
    );

    const nested = effective.scenes.board.widgets.grid.config?.widgets as Record<string, Widget>;
    assert.deepEqual(nested["cell-1"].style, {backgroundColor: "gold"});
    assert.equal(nested["cell-0"].style, undefined, "an unpatched sibling is untouched");

    assert.equal(frozenCopy(server), before, "the server layout is shared with the renderer");
});

test("merging shares structure: untouched scenes and widgets come back by reference", () => {
    const server = layout();
    const layer = new OverrideLayer();
    layer.setWidgetStyle("board", "title", {opacity: 0.5});

    const effective = mergeOverrides(server, layer.snapshot());

    assert.notEqual(effective, server);
    assert.equal(effective.scenes.lobby, server.scenes.lobby, "an untouched scene is reused");
    assert.equal(
        effective.scenes.board.widgets.grid,
        server.scenes.board.widgets.grid,
        "an untouched widget subtree is reused",
    );
    assert.notEqual(effective.scenes.board.widgets.title, server.scenes.board.widgets.title);
});

test("an override for a widget that does not exist is simply ignored", () => {
    const server = layout();
    const layer = new OverrideLayer();
    layer.setWidgetStyle("board", "ghost", {opacity: 0.5});
    layer.setSceneStyle("nowhere", {opacity: 0.5});

    const effective = mergeOverrides(server, layer.snapshot());
    assert.equal(effective, server, "nothing matched, so nothing was copied");
});

test("clearForKeyframe drops every patch", () => {
    const layer = new OverrideLayer();
    layer.setBoardStyle({opacity: 0.5});
    layer.setWidgetConfig("board", "title", {text: "x"});

    layer.clearForKeyframe();

    assert.equal(layer.snapshot(), EMPTY_OVERRIDES);
    const server = layout();
    assert.equal(mergeOverrides(server, layer.snapshot()), server);
});
