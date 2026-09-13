import test from "node:test";
import assert from "node:assert/strict";

import {BOARD_SCOPE} from "./events.ts";
import {LuaHost, MAX_PAYLOAD_DEPTH} from "./host-api.ts";
import {LuaRuntime} from "./runtime.ts";

import type {Scope} from "./events.ts";
import type {Game} from "../../models/models.ts";
import type {LuaResult} from "./runtime.ts";

const SCENE: Scope = {kind: "scene", sceneId: "board"};
const CELL: Scope = {kind: "widget", sceneId: "board", widgetId: "cell-1"};
const OTHER_CELL: Scope = {kind: "widget", sceneId: "board", widgetId: "cell-2"};

interface Harness {
    readonly host: LuaHost;
    readonly rt: LuaRuntime;
    /** Everything `indri.send` handed to the injected callback. */
    readonly sent: {action: string; payload: Record<string, unknown>}[];
    readonly logs: string[];
    /** Errors the runtime reported, from any path. */
    readonly errors: string[];
    dispose(): void;
}

function harness(activeSceneId: () => string | undefined = () => "board"): Harness {
    const sent: Harness["sent"] = [];
    const logs: string[] = [];
    const errors: string[] = [];

    const rt = new LuaRuntime({onError: (e) => errors.push(e)});
    const host = new LuaHost(rt, {
        send: (action, payload) => sent.push({action, payload}),
        activeSceneId,
        log: (...args) => logs.push(args.join(" ")),
    });
    expectOk(host.install(), "installing the indri global");

    return {
        host,
        rt,
        sent,
        logs,
        errors,
        dispose: () => {
            host.dispose();
            rt.dispose();
        },
    };
}

function expectOk<T>(result: LuaResult<T>, what: string): T {
    assert.ok(result.ok, `${what} should have succeeded, got: ${result.ok ? "" : result.error}`);
    return result.value;
}

function expectError<T>(result: LuaResult<T>, what: string): string {
    if (result.ok) assert.fail(`${what} should have failed`);
    return result.error;
}

function game(): Game {
    return {
        code: "ABCD",
        players: {
            p1: {name: "Ada", score: 3, connected: true, host: true, controller: false},
            p2: {name: "Bob", score: 1, connected: false, host: false, controller: false},
        },
        stage: {currentScene: "board", sceneOrder: ["board"]},
        data: {board: {cells: ["X", "", "O"]}},
    };
}

// ---- 1. presentation writes land in the override layer ---------------------

test("widget setStyle and setConfig write to the override map, not to game state", () => {
    const h = harness();
    const source = game();
    h.host.applyGameState(source, "keyframe");

    expectOk(h.host.instantiate(SCENE, `
        indri.widget("cell-1").setStyle({backgroundColor = "gold"})
        indri.widget("cell-1").setConfig({text = "X"})
    `), "a scene chunk painting a widget");

    assert.deepEqual(h.host.overrides.snapshot().widgets, {
        board: {"cell-1": {style: {backgroundColor: "gold"}, config: {text: "X"}}},
    });

    assert.deepEqual(h.host.api.state(), source, "the game state must be byte-identical");
    assert.deepEqual(source, game(), "…and the caller's own object untouched");
    h.dispose();
});

test("a widget script resolves its own scene, not the active one", () => {
    const h = harness(() => "lobby");
    expectOk(h.host.instantiate(CELL, `indri.widget("cell-1").setStyle({opacity = 0.5})`), "a widget chunk");

    assert.deepEqual(Object.keys(h.host.overrides.snapshot().widgets), ["board"]);
    h.dispose();
});

test("board and scene styles reach their own scopes", () => {
    const h = harness();
    expectOk(h.host.instantiate(BOARD_SCOPE, `
        indri.board.setStyle({backgroundColor = "black"})
        indri.scene.setStyle({opacity = 0.5})
    `), "a board chunk");

    assert.deepEqual(h.host.overrides.snapshot().board, {style: {backgroundColor: "black"}});
    assert.deepEqual(h.host.overrides.snapshot().scenes, {board: {style: {opacity: 0.5}}});
    h.dispose();
});

test("a scene-scoped write with no active scene is dropped with a log, not a crash", () => {
    const h = harness(() => undefined);
    expectOk(h.host.instantiate(BOARD_SCOPE, `indri.scene.setStyle({opacity = 0.5})`), "a board chunk");

    assert.deepEqual(h.host.overrides.snapshot().scenes, {});
    assert.equal(h.logs.length, 1);
    assert.match(h.logs[0], /no scene is active/);
    h.dispose();
});

test("an invalid style is rejected at the boundary with a message naming the key", () => {
    const h = harness();
    const error = expectError(
        h.host.instantiate(SCENE, `indri.widget("cell-1").setStyle({nope = 1})`),
        "a chunk setting an unknown style key",
    );

    assert.match(error, /nope/);
    assert.deepEqual(h.host.overrides.snapshot().widgets, {}, "nothing partial is written");
    h.dispose();
});

// ---- 3. keyframe clears overrides; a delta does not ------------------------

test("a keyframe clears overrides and a delta leaves them alone", () => {
    const h = harness();
    h.host.applyGameState(game(), "keyframe");
    expectOk(h.host.instantiate(SCENE, `indri.widget("cell-1").setStyle({backgroundColor = "gold"})`), "a chunk");

    const painted = h.host.overrides.snapshot();
    assert.deepEqual(painted.widgets.board["cell-1"].style, {backgroundColor: "gold"});

    h.host.applyGameState(game(), "delta");
    assert.equal(
        h.host.overrides.snapshot(),
        painted,
        "a delta is incremental — wiping the script's work every tick would make scripts useless",
    );

    h.host.applyGameState(game(), "keyframe");
    assert.deepEqual(
        h.host.overrides.snapshot().widgets,
        {},
        "a keyframe is a resync — presentation computed from stale state must not survive it",
    );
    h.dispose();
});

// ---- 4. bubbling ------------------------------------------------------------

test("a press bubbles widget -> scene -> board", () => {
    const h = harness();
    const order = `indri.on("widgetPress", function(e) indri.log(e.tag) end)`;
    expectOk(h.host.instantiate(BOARD_SCOPE, order.replace("e.tag", `"board"`)), "board chunk");
    expectOk(h.host.instantiate(SCENE, order.replace("e.tag", `"scene"`)), "scene chunk");
    expectOk(h.host.instantiate(CELL, order.replace("e.tag", `"widget"`)), "widget chunk");

    const outcome = h.host.emit("widgetPress", CELL, {id: "cell-1"});

    assert.deepEqual(h.logs, ["widget", "scene", "board"]);
    assert.deepEqual(outcome.errors, []);
    assert.equal(outcome.invoked, 3);
    h.dispose();
});

test("returning false stops the chain; returning nothing does not", () => {
    const h = harness();
    expectOk(h.host.instantiate(BOARD_SCOPE, `
        indri.on("widgetPress", function() indri.log("board") end)
    `), "board chunk");
    expectOk(h.host.instantiate(SCENE, `
        indri.on("widgetPress", function() indri.log("scene") return false end)
    `), "scene chunk");
    expectOk(h.host.instantiate(CELL, `
        indri.on("widgetPress", function() indri.log("widget") end)
    `), "widget chunk");

    const outcome = h.host.emit("widgetPress", CELL);

    assert.deepEqual(h.logs, ["widget", "scene"], "the widget returned nil and did not stop it");
    assert.equal(outcome.stopped, true);
    h.dispose();
});

test("the event payload arrives as a Lua table", () => {
    const h = harness();
    expectOk(h.host.instantiate(CELL, `
        indri.on("widgetPress", function(e) indri.log(e.id, tostring(e.count)) end)
    `), "widget chunk");

    h.host.emit("widgetPress", CELL, {id: "cell-1", count: 2});
    assert.deepEqual(h.logs, ["cell-1 2"]);
    h.dispose();
});

// ---- 5. removed widgets -----------------------------------------------------

test("a handler registered for a removed widget is dropped", () => {
    const h = harness();
    const chunk = (tag: string) => `indri.on("widgetPress", function() indri.log("${tag}") end)`;
    expectOk(h.host.instantiate(SCENE, chunk("scene")), "scene chunk");
    expectOk(h.host.instantiate(CELL, chunk("cell-1")), "widget chunk");
    expectOk(h.host.instantiate(OTHER_CELL, chunk("cell-2")), "sibling widget chunk");

    // A delta removed cell-1; the layout now contains only these scopes.
    h.host.retain([BOARD_SCOPE, SCENE, OTHER_CELL]);

    h.host.emit("widgetPress", CELL);
    assert.deepEqual(h.logs, ["scene"], "the removed widget's handler must not run");

    h.logs.length = 0;
    h.host.emit("widgetPress", OTHER_CELL);
    assert.deepEqual(h.logs, ["cell-2", "scene"], "its sibling is unaffected");
    h.dispose();
});

test("re-instantiating a scope drops its old handlers and its old globals", () => {
    const h = harness();
    expectOk(h.host.instantiate(SCENE, `
        counter = 7
        indri.on("widgetPress", function() indri.log("v1 " .. tostring(counter)) end)
    `), "the first version of the scene script");

    h.host.emit("widgetPress", SCENE);
    assert.deepEqual(h.logs, ["v1 7"]);

    // A delta changed the script text: 022/024 calls instantiate again.
    h.logs.length = 0;
    expectOk(h.host.instantiate(SCENE, `
        indri.on("widgetPress", function() indri.log("v2 " .. tostring(counter)) end)
    `), "the replacement scene script");

    h.host.emit("widgetPress", SCENE);
    assert.deepEqual(h.logs, ["v2 nil"], "the old handler is gone and the old globals with it");
    h.dispose();
});

test("one scope's globals are invisible to another, but the shared libraries are not", () => {
    const h = harness();
    expectOk(h.host.instantiate(SCENE, `mine = "scene"`), "the scene chunk");
    expectOk(h.host.instantiate(CELL, `
        indri.log(tostring(mine), string.upper("ok"), tostring(indri ~= nil))
    `), "the widget chunk");

    assert.deepEqual(h.logs, ["nil OK true"]);
    h.dispose();
});

// ---- 6. re-entrancy ---------------------------------------------------------

test("a host callback invoked from inside a handler cannot start a second dispatch", () => {
    const sent: string[] = [];
    const logs: string[] = [];
    const rt = new LuaRuntime();
    let host: LuaHost | undefined;
    let nested: ReturnType<LuaHost["emit"]> | undefined;

    host = new LuaHost(rt, {
        // `send` is the one host callback a handler can reach synchronously.
        // Re-entering dispatch from it is the exact case the guard exists for.
        send: () => {
            sent.push("send");
            nested = host?.emit("widgetPress", CELL);
        },
        activeSceneId: () => "board",
        log: (...args) => logs.push(args.join(" ")),
    });
    expectOk(host.install(), "install");

    expectOk(host.instantiate(CELL, `
        indri.on("widgetPress", function() indri.log("widget") indri.send("poke", {}) end)
    `), "widget chunk");
    expectOk(host.instantiate(SCENE, `
        indri.on("widgetPress", function() indri.log("scene") end)
    `), "scene chunk");

    const outcome = host.emit("widgetPress", CELL);

    assert.deepEqual(sent, ["send"], "the outbound call itself still happened");
    assert.deepEqual(nested, {invoked: 0, stopped: false, reentrant: true, errors: []});
    assert.deepEqual(logs, ["widget", "scene"], "the outer chain ran exactly once");
    assert.equal(outcome.invoked, 2);

    // The guard released, so the next event is delivered normally.
    logs.length = 0;
    assert.equal(host.emit("widgetPress", CELL).invoked, 2);

    host.dispose();
    rt.dispose();
});

// ---- 7. state is a deep-frozen snapshot ------------------------------------

test("state() is a deep-frozen clone, and a Lua write attempt fails", () => {
    const h = harness();
    const source = game();
    h.host.applyGameState(source, "keyframe");

    const snapshot = h.host.api.state();
    assert.notEqual(snapshot, source, "handing out the live object would be a back door");
    assert.deepEqual(snapshot, source);
    assert.ok(Object.isFrozen(snapshot));
    assert.ok(Object.isFrozen(snapshot.players), "freezing must be deep, not shallow");
    assert.ok(Object.isFrozen(snapshot.players?.p1));
    assert.ok(Object.isFrozen(snapshot.data?.board.cells));

    for (const attempt of [
        `indri.state().code = "HACKED"`,
        `indri.state().players.p1.score = 999`,
        `indri.state().players.p1.name = "Mallory"`,
        `indri.state().data.board.cells[1] = "O"`,
        `local s = indri.state() setmetatable(s, {})`,
        `indri.state().brandNew = 1`,
    ]) {
        const error = expectError(h.rt.run(attempt, "attacker"), attempt);
        assert.match(error, /read-only|protected metatable/);
    }

    assert.deepEqual(source, game(), "the object the host was given is untouched");
    assert.deepEqual(h.host.api.state(), game(), "…and so is the snapshot");
    h.dispose();
});

/**
 * The TYPE side of the same rule, and it only means anything under `tsc`.
 *
 * `@ts-expect-error` FAILS THE TYPECHECK IF THE LINE COMPILES, so this is a
 * real assertion rather than a comment: with the old shallow `Readonly<Game>`,
 * `stage` narrowed to a mutable `Stage` and the write below type-checked,
 * leaving a script author to discover the freeze as a runtime TypeError. The
 * `assert.throws` keeps the two halves honest — the type must refuse it and the
 * runtime must too.
 */
test("a nested write to state() is a type error, not just a runtime one", () => {
    const h = harness();
    h.host.applyGameState(game(), "keyframe");

    const stage = h.host.api.state().stage;
    assert.ok(stage !== undefined);

    assert.throws(
        () => {
            // @ts-expect-error state() is deeply readonly
            stage.currentScene = "results";
        },
        TypeError,
    );

    assert.equal(h.host.api.state().stage?.currentScene, "board");
    h.dispose();
});

test("a script can read every corner of the state it cannot write", () => {
    const h = harness();
    h.host.applyGameState(game(), "keyframe");

    expectOk(h.rt.run(`
        local s = indri.state()
        assert(s.code == "ABCD", "scalar")
        assert(s.players.p1.score == 3, "nested scalar")
        assert(s.players.p2.connected == false, "false is not nil")
        assert(s.data.board.cells[3] == "O", "arrays are 1-based")
        assert(#s.data.board.cells == 3, "__len reaches the backing table")
        local names = {}
        for id, p in pairs(s.players) do names[#names + 1] = id end
        table.sort(names)
        assert(table.concat(names, ",") == "p1,p2", "pairs reaches the backing table")
        assert(getmetatable(s) == false, "the protection cannot be inspected")
    `, "reader"), "a chunk reading state");
    h.dispose();
});

test("the Lua view of state is rebuilt when the state changes", () => {
    const h = harness();
    h.host.applyGameState(game(), "keyframe");
    expectOk(h.rt.run(`assert(indri.state().code == "ABCD")`, "before"), "reading the first state");

    h.host.applyGameState({...game(), code: "WXYZ"}, "delta");
    expectOk(h.rt.run(`assert(indri.state().code == "WXYZ")`, "after"), "reading the second state");
    h.dispose();
});

test("stateChanged fires with the new state on every applied update", () => {
    const h = harness();
    expectOk(h.host.instantiate(SCENE, `
        indri.on("stateChanged", function(s) indri.log("saw " .. tostring(s.code)) end)
    `), "scene chunk");

    h.host.applyGameState(game(), "keyframe");
    h.host.applyGameState({...game(), code: "WXYZ"}, "delta");

    assert.deepEqual(h.logs, ["saw ABCD", "saw WXYZ"]);
    h.dispose();
});

// ---- 8. outbound send, and the absence of an inbound path -------------------

test("send reaches the injected callback with the action and a plain payload", () => {
    const h = harness();
    expectOk(h.host.instantiate(SCENE, `
        indri.send("move", {cell = 3, by = "p1", flags = {true, false}, meta = {["深"] = "utf8"}})
    `), "a chunk sending an action");

    assert.deepEqual(h.sent, [{
        action: "move",
        payload: {cell: 3, by: "p1", flags: [true, false], meta: {"深": "utf8"}},
    }]);
    assert.equal(
        JSON.stringify(h.sent[0].payload),
        `{"cell":3,"by":"p1","flags":[true,false],"meta":{"深":"utf8"}}`,
        "the payload must already be JSON-safe when it leaves Lua",
    );
    h.dispose();
});

test("send accepts a missing payload and rejects a non-table one", () => {
    const h = harness();
    expectOk(h.rt.run(`indri.send("refresh")`, "no-payload"), "send with no payload");
    assert.deepEqual(h.sent, [{action: "refresh", payload: {}}]);

    assert.match(expectError(h.rt.run(`indri.send("x", 5)`, "bad"), "a scalar payload"), /must be a table/);
    assert.match(expectError(h.rt.run(`indri.send()`, "bad"), "no action"), /non-empty string/);
    h.dispose();
});

test("a payload Lua cannot marshal is rejected rather than thrown from a callback", () => {
    const h = harness();

    assert.match(
        expectError(h.rt.run(`indri.send("x", {fn = print})`, "function"), "a function in a payload"),
        /can be passed to the host/,
    );

    // A self-referential table reaches the depth cap instead of hanging or
    // blowing up inside JSON.stringify.
    assert.match(
        expectError(h.rt.run(`local t = {} t.self = t indri.send("x", t)`, "cycle"), "a cyclic table"),
        new RegExp(`${MAX_PAYLOAD_DEPTH} levels deep`),
    );

    assert.deepEqual(h.sent, [], "nothing partial escaped to the socket");
    h.dispose();
});

test("there is no inbound path: no message event, and nothing to receive with", () => {
    const h = harness();

    assert.match(
        expectError(h.rt.run(`indri.on("message", function() end)`, "inbound"), "registering for messages"),
        /is not an event/,
    );

    expectOk(h.rt.run(`
        local names = {}
        for k in pairs(indri) do names[#names + 1] = k end
        table.sort(names)
        assert(table.concat(names, ",") == "board,log,on,scene,send,state,widget",
            "unexpected surface: " .. table.concat(names, ","))
    `, "surface"), "enumerating the indri table");
    h.dispose();
});

// ---- 9. a failing handler is contained --------------------------------------

test("a script error inside a handler is contained and the chain keeps going", () => {
    const h = harness();
    expectOk(h.host.instantiate(BOARD_SCOPE, `
        indri.on("widgetPress", function() indri.log("board") end)
    `), "board chunk");
    expectOk(h.host.instantiate(CELL, `
        indri.on("widgetPress", function() error("boom") end)
    `), "a widget chunk whose handler explodes");

    const outcome = h.host.emit("widgetPress", CELL);

    assert.equal(outcome.errors.length, 1);
    assert.match(outcome.errors[0], /boom/);
    assert.match(outcome.errors[0], /widget:board:cell-1:2:/, "the chunk name locates the failure");
    assert.deepEqual(h.logs, ["board"], "one broken script must not mute the ones above it");
    assert.equal(outcome.stopped, false);

    // The runtime is not poisoned: the same handler fails again, cleanly, and
    // unrelated chunks still run.
    assert.equal(h.host.emit("widgetPress", CELL).errors.length, 1);
    expectOk(h.rt.run(`assert(1 + 1 == 2)`, "after"), "a plain chunk after a handler failure");
    h.dispose();
});

test("a handler that runs away is stopped by the instruction budget, not by hanging", () => {
    const rt = new LuaRuntime({instructionBudget: 5_000});
    const host = new LuaHost(rt, {send: () => {}, activeSceneId: () => "board"});
    expectOk(host.install(), "install");
    expectOk(host.instantiate(CELL, `indri.on("widgetPress", function() while true do end end)`), "chunk");

    const outcome = host.emit("widgetPress", CELL);

    assert.equal(outcome.errors.length, 1);
    assert.match(outcome.errors[0], /instruction budget/);
    host.dispose();
    rt.dispose();
});

test("each handler gets its own instruction budget", () => {
    // One loop costs more than 1_000 instructions (see runtime.node-test.ts),
    // so two handlers sharing a 2_000 budget would starve the second one.
    const chunk = `indri.on("widgetPress", function() local s = 0 for i = 1, 700 do s = s + i end indri.log("ran") end)`;
    const logs: string[] = [];
    const rt = new LuaRuntime({instructionBudget: 2_000});
    const host = new LuaHost(rt, {
        send: () => {},
        activeSceneId: () => "board",
        log: (...args) => logs.push(args.join(" ")),
    });
    expectOk(host.install(), "install");
    expectOk(host.instantiate(CELL, chunk), "widget chunk");
    expectOk(host.instantiate(SCENE, chunk), "scene chunk");
    expectOk(host.instantiate(BOARD_SCOPE, chunk), "board chunk");

    const outcome = host.emit("widgetPress", CELL);

    assert.deepEqual(outcome.errors, []);
    assert.deepEqual(logs, ["ran", "ran", "ran"]);
    host.dispose();
    rt.dispose();
});

// ---- lifecycle --------------------------------------------------------------

test("install is idempotent and a chunk that fails to load registers nothing", () => {
    const h = harness();
    expectOk(h.host.install(), "installing twice");

    assert.match(expectError(h.host.instantiate(SCENE, "return ("), "a syntax error"), /unexpected symbol/);
    assert.equal(h.host.emit("widgetPress", CELL).invoked, 0);
    h.dispose();
});

test("everything still returns a value after the runtime is disposed", () => {
    const h = harness();
    expectOk(h.host.instantiate(SCENE, `indri.on("widgetPress", function() end)`), "a chunk");

    h.rt.dispose();

    const outcome = h.host.emit("widgetPress", SCENE);
    assert.equal(outcome.invoked, 1, "the registration survives; only the call fails");
    assert.match(outcome.errors[0], /disposed/);

    assert.match(expectError(h.host.instantiate(SCENE, "return 1"), "instantiate after dispose"), /disposed/);
    assert.doesNotThrow(() => h.host.dispose());
});
