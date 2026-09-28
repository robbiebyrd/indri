import test from "node:test";
import assert from "node:assert/strict";

import {WebSocketTransportClient} from "./websocket.ts";
import {captureWarnings, FakeWebSocket, record} from "../test-fakes.ts";

function client() {
    const c = new WebSocketTransportClient({url: "ws://server/ws", WebSocket: FakeWebSocket});
    return {c, log: record(c)};
}

test("connect resolves once the socket opens, and asks for binary as ArrayBuffer", async () => {
    const {c, log} = client();
    const connected = c.connect();
    const ws = FakeWebSocket.last();

    assert.equal(ws.url, "ws://server/ws");
    assert.equal(ws.binaryType, "arraybuffer");

    ws.serverOpen();
    await connected;
    assert.equal(log.opens, 1);
});

test("text arrives as a string and binary as a Uint8Array", async () => {
    const {c, log} = client();
    const connected = c.connect();
    const ws = FakeWebSocket.last();
    ws.serverOpen();
    await connected;

    ws.serverSend('{"stage":{"currentScene":"login"}}');
    ws.serverSend(new Uint8Array([0x81, 0xa1]).buffer);

    assert.deepEqual(log.messages, ['{"stage":{"currentScene":"login"}}', new Uint8Array([0x81, 0xa1])]);
});

test("send before open is dropped with a warning; after open it reaches the socket", async () => {
    const {c} = client();
    const connected = c.connect();
    const ws = FakeWebSocket.last();

    const warnings = await captureWarnings(() => c.send("early"));
    assert.equal(ws.sent.length, 0);
    assert.equal(warnings.length, 1);

    ws.serverOpen();
    await connected;
    c.send("{}");
    c.send(new Uint8Array([1, 2]));
    assert.deepEqual(ws.sent, ["{}", new Uint8Array([1, 2])]);
});

test("a server close fires onClose once; later sends are dropped", async () => {
    const {c, log} = client();
    const connected = c.connect();
    const ws = FakeWebSocket.last();
    ws.serverOpen();
    await connected;

    ws.serverClose(1006, "gone");
    ws.serverClose(1006, "gone again");

    assert.equal(log.closes.length, 1);
    await captureWarnings(() => c.send("x"));
    assert.equal(ws.sent.length, 0);
});

test("close() from our side closes the socket without firing onClose", async () => {
    const {c, log} = client();
    const connected = c.connect();
    const ws = FakeWebSocket.last();
    ws.serverOpen();
    await connected;

    c.close();
    ws.serverClose();

    assert.ok(ws.closed);
    assert.equal(log.closes.length, 0);
});

test("a socket that closes before opening rejects connect instead of firing onClose", async () => {
    const {c, log} = client();
    const connected = c.connect();
    FakeWebSocket.last().serverClose(1006, "refused");

    await assert.rejects(connected);
    assert.equal(log.closes.length, 0);
    assert.equal(log.opens, 0);
});

test("connect can only be called once", async () => {
    const {c} = client();
    void c.connect().catch(() => {});
    await assert.rejects(c.connect());
    c.close();
});

test("a missing WebSocket implementation is a clear error", () => {
    const original = globalThis.WebSocket;
    // @ts-expect-error simulating a runtime without WebSocket
    delete globalThis.WebSocket;
    try {
        assert.throws(() => new WebSocketTransportClient({url: "ws://x"}), /WebSocket/);
    } finally {
        globalThis.WebSocket = original;
    }
});
