// transport.ts splits the wire-level seam out of MessageHandler into a
// ClientTransport interface plus the concrete WebSocketTransport, mirroring
// the server's transport.Transport split (see plans/webrtc-transport.md,
// Step 10) so a second transport (WebRTC, Step 11) has somewhere to plug in.
//
// Two units are covered here:
//   - WebSocketTransport itself: does it still gate sends on readyState and
//     still hand inbound frames to onMessage, verbatim?
//   - MessageHandler driving an arbitrary ClientTransport: does it forward
//     connect/send to the interface rather than a raw WebSocket, and does an
//     inbound message still reach the existing parsers array unchanged?
import test from "node:test";
import assert from "node:assert/strict";

import {WebSocketTransport} from "./transport.ts";
import {MessageHandler} from "./message-handler.ts";

import type {ClientTransport} from "./transport.ts";

/** A WebSocket stand-in whose open is driven by the test. */
class FakeSocket {
    static instances: FakeSocket[] = [];
    // WebSocketTransport.send compares readyState against WebSocket.OPEN, so
    // the stand-in has to carry the same constants or every send looks closed.
    static readonly CONNECTING = 0;
    static readonly OPEN = 1;
    static readonly CLOSING = 2;
    static readonly CLOSED = 3;
    onopen: (() => void) | null = null;
    onmessage: ((e: {data: string}) => void) | null = null;
    onerror: ((e: unknown) => void) | null = null;
    onclose: ((e: unknown) => void) | null = null;
    readyState = 0;
    sent: string[] = [];
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

function withFakeSocket<T>(fn: (latest: () => FakeSocket) => T): T {
    const g = globalThis as {WebSocket?: unknown};
    const original = g.WebSocket;
    FakeSocket.instances = [];
    g.WebSocket = FakeSocket as unknown as typeof WebSocket;
    try {
        return fn(() => FakeSocket.instances[FakeSocket.instances.length - 1]);
    } finally {
        g.WebSocket = original;
    }
}

test("WebSocketTransport drops a send before the socket is open, with a warning", () => {
    withFakeSocket((latest) => {
        const transport = new WebSocketTransport();
        transport.connect("ws://test");

        const warn = console.warn;
        let warned = 0;
        console.warn = () => {warned++};
        try {
            transport.send({action: "reconnect", sessionId: "t"});
        } finally {
            console.warn = warn;
        }

        assert.equal(latest().sent.length, 0, "a pre-handshake send is dropped");
        assert.equal(warned, 1, "the drop is warned about");
    });
});

test("WebSocketTransport sends once the socket is open", () => {
    withFakeSocket((latest) => {
        const transport = new WebSocketTransport();
        transport.connect("ws://test");
        latest().open();

        transport.send({action: "reconnect", sessionId: "t"});

        assert.deepStrictEqual(
            JSON.parse(latest().sent[0]),
            {action: "reconnect", sessionId: "t"},
        );
    });
});

test("WebSocketTransport hands inbound data to the onMessage handler", () => {
    withFakeSocket((latest) => {
        const transport = new WebSocketTransport();
        transport.connect("ws://test");

        const received: string[] = [];
        transport.onMessage((data) => received.push(data));

        latest().onmessage?.({data: '{"op":"update"}'});

        assert.deepStrictEqual(received, ['{"op":"update"}']);
    });
});

/** A minimal ClientTransport stand-in — no WebSocket involved at all. */
class FakeTransport implements ClientTransport {
    connectedTo?: string;
    sent: object[] = [];
    private messageHandler?: (data: string) => void;

    connect(url: string): void {
        this.connectedTo = url;
    }

    send(message: object): void {
        this.sent.push(message);
    }

    onMessage(handler: (data: string) => void): void {
        this.messageHandler = handler;
    }

    onOpen(_handler: () => void): void {
        // Not exercised here: these tests care about send/receive wiring,
        // not the open lifecycle (covered in message-handler.node-test.ts).
    }

    close(): void {
        // Nothing to release.
    }

    /** Test-only: deliver a raw inbound frame as a real transport would. */
    deliver(data: string): void {
        this.messageHandler?.(data);
    }
}

test("MessageHandler drives the ClientTransport interface instead of constructing a WebSocket", () => {
    const noop = () => undefined;
    const transport = new FakeTransport();

    const handler = new MessageHandler("ws://test", noop, noop, noop, [], transport);

    assert.equal(transport.connectedTo, "ws://test", "the transport is connected to the given url");

    handler.send({action: "ping"});
    assert.deepStrictEqual(transport.sent, [{action: "ping"}], "send is forwarded to the transport, not a socket");
});

test("an inbound message reaches the existing parsers array unchanged", () => {
    const noop = () => undefined;
    const transport = new FakeTransport();
    let received: unknown;

    const parsers = [{
        name: "test_customAction",
        action: "customAction",
        parser: (d: unknown) => {received = d},
    }];

    new MessageHandler("ws://test", noop, noop, noop, parsers, transport);

    transport.deliver(JSON.stringify({op: "customAction", foo: "bar"}));

    assert.deepStrictEqual(received, {op: "customAction", foo: "bar"});
});
