// The bridge is the one place the websocket and the Lua runtime meet, so these
// tests drive the REAL `MessageHandler` over a fake socket rather than a stand-in
// for it. Two of the properties under test — a send before the socket is open is
// dropped, and no parser was added — are properties of that class, and a fake
// would only restate them.
import test from "node:test";
import assert from "node:assert/strict";

import {LuaBridge, collectScripts} from "./bridge.ts";
import {LUA_EVENTS} from "./events.ts";
import {MessageHandler} from "../../services/message-handler.ts";

/** Every socket `MessageHandler` has opened, newest last. */
const opened: FakeSocket[] = [];

class FakeSocket {
    static readonly CONNECTING = 0;
    static readonly OPEN = 1;
    static readonly CLOSING = 2;
    static readonly CLOSED = 3;

    readyState: number = FakeSocket.CONNECTING;
    onmessage: ((e: {data: string}) => void) | null = null;
    onerror: ((e: unknown) => void) | null = null;
    onclose: ((e: unknown) => void) | null = null;
    readonly sent: string[] = [];

    readonly url: string;

    constructor(url: string) {
        this.url = url;
        opened.push(this);
    }

    send(data: string): void {
        this.sent.push(data);
    }

    close(): void {
        this.readyState = FakeSocket.CLOSED;
    }
}

// `MessageHandler` reads the global at construction time and reads
// `WebSocket.OPEN` on every send, so both have to come from the fake.
Object.defineProperty(globalThis, "WebSocket", {
    value: FakeSocket,
    writable: true,
    configurable: true,
});

const KEYFRAME_AT = "2020-01-01T00:00:00.000Z";

interface Harness {
    readonly ws: MessageHandler;
    readonly socket: FakeSocket;
    readonly bridge: LuaBridge;
    /** Everything `indri.log` produced, in order. */
    readonly logs: string[];
    /** Every script failure the runtime reported. */
    readonly errors: string[];
    /** Messages the socket actually transmitted, parsed back. */
    sent(): Record<string, unknown>[];
    /** Deliver a keyframe for `game`. */
    keyframe(game: Record<string, unknown>): void;
    /** Deliver a delta of dotted-path updates. */
    delta(updated: Record<string, unknown>, ts: string): void;
    /** Deliver an arbitrary websocket message. */
    raw(message: Record<string, unknown>): void;
    dispose(): void;
}

function harness(opts: {open?: boolean} = {}): Harness {
    const logs: string[] = [];
    const errors: string[] = [];
    const noop = () => undefined;

    const ws = new MessageHandler("ws://test", noop, noop, noop);
    const socket = opened[opened.length - 1];
    if (opts.open !== false) socket.readyState = FakeSocket.OPEN;

    const bridge = new LuaBridge({
        socket: ws,
        state: ws,
        log: (...args) => logs.push(args.join(" ")),
        onError: (e) => errors.push(e),
    });

    const deliver = (message: Record<string, unknown>) => {
        ws.routeIncomingMessage({data: JSON.stringify(message)} as MessageEvent);
    };

    return {
        ws,
        socket,
        bridge,
        logs,
        errors,
        sent: () => socket.sent.map((s) => JSON.parse(s) as Record<string, unknown>),
        keyframe: (game) => deliver({id: "game-1", ...game}),
        delta: (updated, ts) => deliver({
            op: "update",
            id: "game-1",
            ts,
            type: "game",
            updated,
            removed: [],
        }),
        raw: deliver,
        dispose: () => {
            bridge.dispose();
            ws.close();
        },
    };
}

interface Scripts {
    board?: string;
    scene?: string;
    cell?: string;
}

/** A minimal game carrying a layout, with scripts at whichever levels are named. */
function gameWith(scripts: Scripts, updatedAt: string = KEYFRAME_AT): Record<string, unknown> {
    return {
        code: "ABCD",
        updatedAt,
        stage: {currentScene: "board", sceneOrder: ["board"], scenes: {board: {}}},
        data: {
            layout: {
                grid: {cols: 3, rows: 3},
                ...(scripts.board === undefined ? {} : {script: scripts.board}),
                scenes: {
                    board: {
                        ...(scripts.scene === undefined ? {} : {script: scripts.scene}),
                        widgets: {
                            cell: {
                                type: "text",
                                placement: {kind: "grid", col: 0, row: 0, w: 1, h: 1},
                                ...(scripts.cell === undefined ? {} : {script: scripts.cell}),
                            },
                        },
                    },
                },
            },
        },
    };
}

/** Run `body` with `console.warn` captured. */
function captureWarnings<T>(body: () => T): {value: T; warnings: string[]} {
    const warnings: string[] = [];
    const original = console.warn;
    console.warn = (...args: unknown[]) => {
        warnings.push(args.map((a) => String(a)).join(" "));
    };
    try {
        return {value: body(), warnings};
    } finally {
        console.warn = original;
    }
}

// ---- 1. outbound ----------------------------------------------------------

test("indri.send reaches the socket as one {action, ...payload} message", () => {
    const h = harness();
    h.keyframe(gameWith({scene: `indri.send("move", {move = "1,2", by = 7})`}));

    assert.deepEqual(h.errors, []);
    assert.deepEqual(h.sent(), [{action: "move", move: "1,2", by: 7}]);
    h.dispose();
});

test("a payload key called action cannot rename the message", () => {
    const h = harness();
    h.keyframe(gameWith({scene: `indri.send("move", {action = "kick", who = "p1"})`}));

    assert.deepEqual(h.errors, []);
    assert.deepEqual(
        h.sent(),
        [{action: "move", who: "p1"}],
        "the action a script asked for is the action the server must see",
    );
    h.dispose();
});

test("a send with no payload is still a well-formed message", () => {
    const h = harness();
    h.keyframe(gameWith({scene: `indri.send("refresh")`}));

    assert.deepEqual(h.sent(), [{action: "refresh"}]);
    h.dispose();
});

// ---- 2. a send before the socket is open ----------------------------------

test("a send before the socket is open is dropped with a warning, not thrown", () => {
    const {value: h, warnings} = captureWarnings(() => {
        const inner = harness({open: false});
        inner.keyframe(gameWith({scene: `indri.send("move", {move = "0,0"})`}));
        return inner;
    });

    assert.deepEqual(h.sent(), [], "nothing may be transmitted over a closed socket");
    assert.deepEqual(h.errors, [], "the drop is the socket's business, not a script error");
    assert.equal(warnings.length, 1);
    assert.match(warnings[0], /dropping message sent before the socket was open/);
    h.dispose();
});

// ---- 3. state observation -------------------------------------------------

test("each applied update fires stateChanged exactly once, from the reduced state", () => {
    const h = harness();
    h.keyframe(gameWith({
        scene: `
            count = 0
            indri.on("stateChanged", function(g)
                count = count + 1
                indri.log("state", count, g.code)
            end)
        `,
    }));

    assert.deepEqual(h.logs, ["state 1 ABCD"], "the keyframe that installed the handler counts");

    h.delta({code: "WXYZ"}, "2020-01-01T00:00:01.000Z");
    assert.deepEqual(
        h.logs,
        ["state 1 ABCD", "state 2 WXYZ"],
        "the script sees the reduced game, not the raw delta",
    );

    h.delta({"players.p1.score": 4}, "2020-01-01T00:00:02.000Z");
    assert.deepEqual(h.logs.length, 3);
    assert.deepEqual(h.errors, []);
    h.dispose();
});

test("state that never produced a game does not reach Lua", () => {
    const h = harness();
    // A delta before any keyframe: GameStateParser holds it, `current()` is
    // still undefined, and there is nothing to observe.
    h.delta({code: "WXYZ"}, "2020-01-01T00:00:01.000Z");

    assert.deepEqual(h.logs, []);
    assert.deepEqual(h.sent(), []);
    h.dispose();
});

// ---- 3b. sceneChanged -----------------------------------------------------

const WATCH_SCENE = `
    indri.on("stateChanged", function(g) indri.log("state", g.stage.currentScene) end)
    indri.on("sceneChanged", function(id) indri.log("scene", id) end)
`;

test("sceneChanged fires only when currentScene moves, and after stateChanged", () => {
    const h = harness();
    h.keyframe(gameWith({scene: WATCH_SCENE}));

    assert.deepEqual(
        h.logs,
        ["state board", "scene board"],
        "the first keyframe is a scene change, and state is already current when it lands",
    );

    // A delta that changes something else must not re-announce the scene.
    h.delta({code: "WXYZ"}, "2020-01-01T00:00:01.000Z");
    assert.deepEqual(h.logs, ["state board", "scene board", "state board"]);

    h.delta({"stage.currentScene": "results"}, "2020-01-01T00:00:02.000Z");
    assert.deepEqual(
        h.logs,
        ["state board", "scene board", "state board", "state results", "scene results"],
        "a handler told the scene is now X must find a state that already says X",
    );

    assert.deepEqual(h.errors, []);
    h.dispose();
});

test("a keyframe for the same scene does not re-announce it", () => {
    const h = harness();
    h.keyframe(gameWith({scene: WATCH_SCENE}));
    h.logs.length = 0;

    h.keyframe(gameWith({scene: WATCH_SCENE}, "2020-01-01T00:00:01.000Z"));

    assert.deepEqual(h.logs, ["state board"], "nothing moved, so no scene change happened");
    h.dispose();
});

// ---- 4. keyframe clears overrides, delta does not -------------------------

test("a keyframe clears the override layer and a delta leaves it alone", () => {
    const h = harness();
    // Paints once only, so a surviving override is distinguishable from a
    // repaint. Painting happens in the handler, not in the chunk body: the
    // chunk runs before the new snapshot lands (see LuaBridge.apply).
    const script = `
        painted = false
        indri.on("stateChanged", function()
            if not painted then
                painted = true
                indri.board.setStyle({ backgroundColor = "#123456" })
            end
        end)
    `;

    h.keyframe(gameWith({scene: script}));
    const board = () => h.bridge.host.overrides.snapshot().board.style?.backgroundColor;
    assert.equal(board(), "#123456");

    h.delta({code: "WXYZ"}, "2020-01-01T00:00:01.000Z");
    assert.equal(board(), "#123456", "a delta is incremental; it must not wipe a script's work");

    // Same source, so the chunk is not re-instantiated and `painted` stays true.
    h.keyframe(gameWith({scene: script}, "2020-01-01T00:00:02.000Z"));
    assert.equal(board(), undefined, "a keyframe is a resync; stale presentation must not survive");

    assert.deepEqual(h.errors, []);
    h.dispose();
});

// ---- 5. there is no inbound path ------------------------------------------

test("the bridge registers no parser and routes no message into Lua", () => {
    const h = harness();

    assert.deepEqual(
        parserNames(h.ws),
        ["indri_authenticated", "indri_inquiryResponse", "indri_keyframe", "indri_update"],
        "attaching a bridge must not add an inbound parser",
    );

    assert.ok(
        !(LUA_EVENTS as readonly string[]).includes("message"),
        "a 'message' event would let a script disagree with the reducer",
    );

    h.keyframe(gameWith({
        scene: `
            indri.on("stateChanged", function() indri.log("stateChanged") end)
            indri.on("sceneChanged", function() indri.log("sceneChanged") end)
            indri.on("widgetPress", function() indri.log("widgetPress") end)
        `,
    }));
    // Both of these are announcements the bridge makes about reduced state.
    // `widgetPress` is absent because no one pressed anything, which is the
    // point: nothing here is driven by an incoming message.
    assert.deepEqual(h.logs, ["stateChanged", "sceneChanged"]);
    h.logs.length = 0;

    // Messages that do not produce new game state must reach no script at all.
    h.raw({authenticated: true, user: {id: "u1"}, sessionId: "deadbeef"});
    h.raw({op: "inquiryResponse", games: []});
    h.raw({op: "somethingTheClientHasNeverHeardOf", payload: {a: 1}});
    h.raw({greeting: "hello"});

    assert.deepEqual(h.logs, [], "no websocket message is dispatched to Lua");
    assert.deepEqual(h.errors, []);
    h.dispose();
});

/** `parsers` is private to `MessageHandler`; this test exists to watch it. */
function parserNames(ws: MessageHandler): string[] {
    return (ws as unknown as {parsers: {name: string}[]}).parsers.map((p) => p.name);
}

// ---- 6. payloads that cannot be serialised --------------------------------

test("a cyclic payload is rejected instead of throwing inside the Lua callback", () => {
    const h = harness();
    h.keyframe(gameWith({
        scene: `
            local t = {}
            t.self = t
            indri.send("move", t)
        `,
    }));

    assert.deepEqual(h.sent(), [], "nothing reaches JSON.stringify");
    assert.equal(h.errors.length, 1);
    assert.match(h.errors[0], /nested more than 8 levels deep/);
    h.dispose();
});

test("a payload nested past the depth cap is rejected", () => {
    const h = harness();
    h.keyframe(gameWith({
        scene: `indri.send("move", {a={b={c={d={e={f={g={h={i=1}}}}}}}}})`,
    }));

    assert.deepEqual(h.sent(), []);
    assert.match(h.errors[0], /nested more than 8 levels deep/);
    h.dispose();
});

// ---- 7. script lifecycle --------------------------------------------------

test("only a script whose source changed is re-instantiated", () => {
    const h = harness();
    const v1 = `indri.log("scene v1")`;

    h.keyframe(gameWith({board: `indri.log("board init")`, scene: v1}));
    assert.deepEqual(h.logs, ["board init", "scene v1"]);

    // A delta that re-sends the identical source must not re-run the chunk:
    // re-running would discard script-local state on every tick.
    h.delta({"data.layout.scenes.board.script": v1}, "2020-01-01T00:00:01.000Z");
    assert.deepEqual(h.logs, ["board init", "scene v1"]);

    h.delta(
        {"data.layout.scenes.board.script": `indri.log("scene v2")`},
        "2020-01-01T00:00:02.000Z",
    );
    assert.deepEqual(
        h.logs,
        ["board init", "scene v1", "scene v2"],
        "only the scope whose source changed re-runs",
    );

    assert.deepEqual(h.errors, []);
    h.dispose();
});

test("a scope whose script the layout dropped stops receiving events", () => {
    const h = harness();
    h.keyframe(gameWith({scene: `indri.on("stateChanged", function() indri.log("scene") end)`}));
    assert.deepEqual(h.logs, ["scene"]);

    // Replace the whole scene with one that has no script.
    h.delta(
        {"data.layout.scenes.board": {widgets: {}}},
        "2020-01-01T00:00:01.000Z",
    );
    assert.deepEqual(h.logs, ["scene"], "the dropped scope's handler is gone");

    h.delta({code: "WXYZ"}, "2020-01-01T00:00:02.000Z");
    assert.deepEqual(h.logs, ["scene"]);
    h.dispose();
});

test("widget scripts run in their own scope and bubble to the scene", () => {
    const h = harness();
    h.keyframe(gameWith({
        scene: `indri.on("widgetPress", function(id) indri.log("scene saw " .. id) end)`,
        cell: `indri.on("widgetPress", function(id) indri.log("cell saw " .. id) end)`,
    }));

    h.bridge.host.emit("widgetPress", {kind: "widget", sceneId: "board", widgetId: "cell"}, "cell");

    assert.deepEqual(h.logs, ["cell saw cell", "scene saw cell"]);
    assert.deepEqual(h.errors, []);
    h.dispose();
});

// ---- lifecycle ------------------------------------------------------------

test("a disposed bridge stops observing and ignores further updates", () => {
    const h = harness();
    h.keyframe(gameWith({scene: `indri.on("stateChanged", function() indri.log("tick") end)`}));
    assert.deepEqual(h.logs, ["tick"]);

    h.bridge.dispose();
    h.delta({code: "WXYZ"}, "2020-01-01T00:00:01.000Z");

    assert.deepEqual(h.logs, ["tick"]);
    assert.doesNotThrow(() => h.bridge.dispose(), "dispose is idempotent");
    h.ws.close();
});

// ---- collectScripts -------------------------------------------------------

test("collectScripts finds every level, including widgets inside sub-grids", () => {
    const scripts = collectScripts({
        script: "board",
        scenes: {
            main: {
                script: "scene",
                widgets: {
                    grid: {
                        type: "subgrid",
                        config: {widgets: {cell: {type: "text", script: "cell"}}},
                    },
                    plain: {type: "text", script: "plain"},
                },
            },
        },
    });

    assert.deepEqual(
        [...scripts.entries()].map(([key, entry]) => [key, entry.src]),
        [
            ["board", "board"],
            ["scene:main", "scene"],
            ["widget:main:cell", "cell"],
            ["widget:main:plain", "plain"],
        ],
    );
});

test("collectScripts tolerates malformed layouts and caps its recursion", () => {
    assert.equal(collectScripts(undefined).size, 0);
    assert.equal(collectScripts("not a layout").size, 0);
    assert.equal(collectScripts({scenes: []}).size, 0);
    assert.equal(collectScripts({script: 42, scenes: {a: null}}).size, 0);
    assert.equal(collectScripts({script: "", scenes: {}}).size, 0, "an empty script is not a script");

    // Six levels of sub-grid; only the four the schema allows are walked.
    let widget: Record<string, unknown> = {type: "text", script: "deepest"};
    for (let depth = 0; depth < 6; depth++) {
        widget = {type: "subgrid", script: `level${depth}`, config: {widgets: {[`w${depth}`]: widget}}};
    }

    const scripts = collectScripts({scenes: {main: {widgets: {root: widget}}}});
    assert.deepEqual(
        [...scripts.keys()],
        ["widget:main:root", "widget:main:w5", "widget:main:w4", "widget:main:w3"],
        "the walk stops at MAX_SUBGRID_DEPTH rather than following hostile nesting",
    );
});
