// Covers the socket-open hook that session restore depends on. The rest of
// MessageHandler is exercised through layout/lua/bridge.node-test.ts.
import test from "node:test";
import assert from "node:assert/strict";

import {MessageHandler} from "./message-handler.ts";

/** A WebSocket stand-in whose open is driven by the test. */
class FakeSocket {
    static instances: FakeSocket[] = [];
    // MessageHandler.send compares readyState against WebSocket.OPEN, so the
    // stand-in has to carry the same constants or every send looks closed.
    static readonly CONNECTING = 0;
    static readonly OPEN = 1;
    static readonly CLOSING = 2;
    static readonly CLOSED = 3;
    onopen: (() => void) | null = null;
    onmessage: ((e: unknown) => void) | null = null;
    onerror: ((e: unknown) => void) | null = null;
    onclose: ((e: unknown) => void) | null = null;
    readyState = 0;
    sent: string[] = [];
    // Declared, not a parameter property: node --experimental-strip-types runs
    // in strip-only mode and cannot desugar `constructor(public url: string)`.
    url: string;

    constructor(url: string) {
        this.url = url;
        FakeSocket.instances.push(this);
    }

    send(data: string): void {
        this.sent.push(data);
    }

    close(): void {
        this.readyState = 3;
    }

    open(): void {
        this.readyState = 1;
        this.onopen?.();
    }
}

function withFakeSocket<T>(fn: () => T): T {
    const g = globalThis as {WebSocket?: unknown};
    const original = g.WebSocket;
    FakeSocket.instances = [];
    g.WebSocket = FakeSocket as unknown as typeof WebSocket;
    try {
        return fn();
    } finally {
        g.WebSocket = original;
    }
}

function newHandler(): {ws: MessageHandler; socket: FakeSocket} {
    const noop = () => undefined;
    const ws = new MessageHandler("ws://test/ws", noop, noop, noop);
    return {ws, socket: FakeSocket.instances[FakeSocket.instances.length - 1]};
}

test("an open observer fires when the socket opens, not before", () => {
    withFakeSocket(() => {
        const {ws, socket} = newHandler();
        let fired = 0;
        ws.onOpen(() => {fired++});

        assert.equal(fired, 0, "nothing may run before the handshake completes");
        socket.open();
        assert.equal(fired, 1);
    });
});

test("an observer registered after the socket is already open fires immediately", () => {
    withFakeSocket(() => {
        const {ws, socket} = newHandler();
        socket.open();

        let fired = 0;
        ws.onOpen(() => {fired++});
        assert.equal(fired, 1, "a late subscriber must not wait for an open that already happened");
    });
});

test("unsubscribing before open stops the observer firing", () => {
    withFakeSocket(() => {
        const {ws, socket} = newHandler();
        let fired = 0;
        const off = ws.onOpen(() => {fired++});

        off();
        socket.open();
        assert.equal(fired, 0, "a provider that unmounted first must not act");
    });
});

test("a throwing observer does not stop the next one", () => {
    withFakeSocket(() => {
        const {ws, socket} = newHandler();
        const warn = console.warn;
        console.warn = () => undefined;
        let second = 0;
        try {
            ws.onOpen(() => {throw new Error("boom")});
            ws.onOpen(() => {second++});
            socket.open();
        } finally {
            console.warn = warn;
        }
        assert.equal(second, 1);
    });
});

// THE WHOLE POINT OF THIS HOOK, once a transport can fail over
// (plans/client-transport-failover.md, Step 5). A switch lands on a brand new
// server-side connection with no session, so session restore has to run again
// or the player is silently logged out mid-game with their game gone. The old
// latch fired observers exactly once and made that impossible.
test("an open observer runs again on every later reconnect", () => {
    withFakeSocket(() => {
        const {ws, socket} = newHandler();
        let fired = 0;
        ws.onOpen(() => {fired++});

        socket.open();
        assert.equal(fired, 1, "the first connect");

        socket.open();
        assert.equal(fired, 2, "and again when the transport reconnects");
    });
});

test("an observer registered while already open still runs on a later reconnect", () => {
    withFakeSocket(() => {
        const {ws, socket} = newHandler();
        socket.open();

        let fired = 0;
        ws.onOpen(() => {fired++});
        assert.equal(fired, 1, "immediately, because the transport is already up");

        socket.open();
        assert.equal(fired, 2, "and again on the reconnect — it was not a one-shot");
    });
});

test("unsubscribing after an open stops the observer firing on a reconnect", () => {
    withFakeSocket(() => {
        const {ws, socket} = newHandler();
        socket.open();

        let fired = 0;
        const off = ws.onOpen(() => {fired++});
        assert.equal(fired, 1);

        off();
        socket.open();
        assert.equal(fired, 1, "a provider that unmounted must not act on a later reconnect");
    });
});

test("an observer that throws when registered late does not stop the next one", () => {
    withFakeSocket(() => {
        const {ws, socket} = newHandler();
        socket.open();

        const warn = console.warn;
        console.warn = () => undefined;
        let second = 0;
        try {
            ws.onOpen(() => {throw new Error("boom")});
            ws.onOpen(() => {second++});
        } finally {
            console.warn = warn;
        }

        assert.equal(second, 1, "the throw did not escape onOpen either");
    });
});

// The reason onOpen exists: send() drops anything written before the
// handshake, so a reconnect fired on mount would restore nothing.
test("send before open is dropped, and lands once open", () => {
    withFakeSocket(() => {
        const {ws, socket} = newHandler();
        const warn = console.warn;
        console.warn = () => undefined;
        try {
            ws.send({action: "reconnect", sessionId: "t"});
        } finally {
            console.warn = warn;
        }
        assert.equal(socket.sent.length, 0, "a pre-handshake send is dropped");

        socket.open();
        ws.onOpen(() => ws.send({action: "reconnect", sessionId: "t"}));
        assert.deepStrictEqual(
            JSON.parse(socket.sent[0]),
            {action: "reconnect", sessionId: "t"},
        );
    });
});
