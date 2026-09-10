import test from "node:test";
import assert from "node:assert/strict";

import {BOARD_SCOPE, EventBus, LUA_EVENTS, bubblePath, isLuaEvent, scopeKey} from "./events.ts";

import type {Invoke, Scope} from "./events.ts";

const SCENE: Scope = {kind: "scene", sceneId: "board"};
const WIDGET: Scope = {kind: "widget", sceneId: "board", widgetId: "cell-1"};

/** Records the order handlers ran in; each handler is its own name. */
function recorder(log: string[], stop: ReadonlySet<string> = new Set()): Invoke<string> {
    return (handler) => {
        log.push(handler);
        return !stop.has(handler);
    };
}

test("scope keys are distinct and round-trip through the bus", () => {
    assert.equal(scopeKey(BOARD_SCOPE), "board");
    assert.equal(scopeKey(SCENE), "scene:board");
    assert.equal(scopeKey(WIDGET), "widget:board:cell-1");
    assert.notEqual(scopeKey(SCENE), scopeKey(WIDGET));
});

test("bubblePath runs widget -> scene -> board", () => {
    assert.deepEqual(bubblePath(WIDGET).map(scopeKey), ["widget:board:cell-1", "scene:board", "board"]);
    assert.deepEqual(bubblePath(SCENE).map(scopeKey), ["scene:board", "board"]);
    assert.deepEqual(bubblePath(BOARD_SCOPE).map(scopeKey), ["board"]);
});

test("only the three known events exist, and there is no inbound message event", () => {
    assert.deepEqual([...LUA_EVENTS], ["stateChanged", "sceneChanged", "widgetPress"]);
    assert.equal(isLuaEvent("message"), false, "Lua observes state; it never receives messages");
    assert.equal(isLuaEvent("widgetPress"), true);
});

test("dispatch bubbles widget -> scene -> board in registration order", () => {
    const bus = new EventBus<string>();
    bus.register(BOARD_SCOPE, "widgetPress", "board-1");
    bus.register(SCENE, "widgetPress", "scene-1");
    bus.register(SCENE, "widgetPress", "scene-2");
    bus.register(WIDGET, "widgetPress", "widget-1");

    const log: string[] = [];
    const result = bus.dispatch("widgetPress", WIDGET, recorder(log));

    assert.deepEqual(log, ["widget-1", "scene-1", "scene-2", "board-1"]);
    assert.deepEqual(result, {invoked: 4, stopped: false, reentrant: false});
});

test("dispatch stops as soon as a handler returns false", () => {
    const bus = new EventBus<string>();
    bus.register(BOARD_SCOPE, "widgetPress", "board-1");
    bus.register(SCENE, "widgetPress", "scene-1");
    bus.register(SCENE, "widgetPress", "scene-2");
    bus.register(WIDGET, "widgetPress", "widget-1");

    const log: string[] = [];
    const result = bus.dispatch("widgetPress", WIDGET, recorder(log, new Set(["scene-1"])));

    assert.deepEqual(log, ["widget-1", "scene-1"], "the rest of the scene and the board are skipped");
    assert.equal(result.stopped, true);
    assert.equal(result.invoked, 2);
});

test("only the named event fires, and only for scopes on the path", () => {
    const bus = new EventBus<string>();
    bus.register(WIDGET, "widgetPress", "press");
    bus.register(WIDGET, "stateChanged", "state");
    bus.register({kind: "widget", sceneId: "board", widgetId: "other"}, "widgetPress", "sibling");

    const log: string[] = [];
    bus.dispatch("widgetPress", WIDGET, recorder(log));

    assert.deepEqual(log, ["press"], "a sibling widget is not on the bubble path");
});

test("broadcast reaches every scope and a false return does not stop it", () => {
    const bus = new EventBus<string>();
    bus.register(BOARD_SCOPE, "stateChanged", "board-1");
    bus.register(SCENE, "stateChanged", "scene-1");
    bus.register(WIDGET, "stateChanged", "widget-1");
    bus.register(WIDGET, "widgetPress", "press");

    const log: string[] = [];
    const result = bus.broadcast("stateChanged", recorder(log, new Set(["board-1"])));

    assert.deepEqual(log.sort(), ["board-1", "scene-1", "widget-1"]);
    assert.equal(result.stopped, false, "no script may suppress another's copy of an announcement");
    assert.equal(result.invoked, 3);
});

test("a handler registered for a removed widget is dropped", () => {
    const bus = new EventBus<string>();
    bus.register(WIDGET, "widgetPress", "doomed");
    bus.register(SCENE, "widgetPress", "survivor");

    const dropped = bus.retain([BOARD_SCOPE, SCENE]);

    assert.deepEqual(dropped, ["doomed"], "the dropped handlers come back so refs can be released");
    assert.equal(bus.has(WIDGET), false);

    const log: string[] = [];
    bus.dispatch("widgetPress", WIDGET, recorder(log));
    assert.deepEqual(log, ["survivor"], "the removed widget's handler must never run again");
});

test("removeScope forgets one scope and leaves its neighbours alone", () => {
    const bus = new EventBus<string>();
    bus.register(SCENE, "stateChanged", "a");
    bus.register(SCENE, "stateChanged", "b");
    bus.register(BOARD_SCOPE, "stateChanged", "board");

    assert.deepEqual(bus.removeScope(SCENE), ["a", "b"]);
    assert.deepEqual(bus.removeScope(SCENE), [], "removing twice is harmless");

    const log: string[] = [];
    bus.broadcast("stateChanged", recorder(log));
    assert.deepEqual(log, ["board"]);
});

test("re-entrancy is refused, not queued", () => {
    const bus = new EventBus<string>();
    const log: string[] = [];
    let inner: ReturnType<EventBus<string>["dispatch"]> | undefined;

    bus.register(WIDGET, "widgetPress", "outer");
    bus.register(BOARD_SCOPE, "widgetPress", "board");

    const reenter: Invoke<string> = (handler) => {
        log.push(handler);
        if (handler === "outer") inner = bus.dispatch("widgetPress", WIDGET, reenter);
        return true;
    };

    const result = bus.dispatch("widgetPress", WIDGET, reenter);

    assert.deepEqual(inner, {invoked: 0, stopped: false, reentrant: true});
    assert.deepEqual(log, ["outer", "board"], "the nested call ran nothing at all");
    assert.equal(result.reentrant, false);

    // The guard is released afterwards, so the next real dispatch works.
    const after: string[] = [];
    assert.equal(bus.dispatch("widgetPress", WIDGET, recorder(after)).invoked, 2);
});

test("a handler registering another handler mid-dispatch does not disturb the walk", () => {
    const bus = new EventBus<string>();
    bus.register(SCENE, "widgetPress", "first");
    bus.register(SCENE, "widgetPress", "second");

    const log: string[] = [];
    const invoke: Invoke<string> = (handler) => {
        log.push(handler);
        if (handler === "first") bus.register(SCENE, "widgetPress", "late");
        return true;
    };

    bus.dispatch("widgetPress", SCENE, invoke);
    assert.deepEqual(log, ["first", "second"], "a handler added mid-walk waits for the next event");

    const next: string[] = [];
    bus.dispatch("widgetPress", SCENE, recorder(next));
    assert.deepEqual(next, ["first", "second", "late"]);
});

test("clear returns every handler so the caller can release them", () => {
    const bus = new EventBus<string>();
    bus.register(BOARD_SCOPE, "stateChanged", "a");
    bus.register(WIDGET, "widgetPress", "b");

    assert.deepEqual(bus.clear().sort(), ["a", "b"]);
    assert.equal(bus.broadcast("stateChanged", () => true).invoked, 0);
});
