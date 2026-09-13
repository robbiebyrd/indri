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

import {WebSocketTransport, connectWithTimeout} from "./transport.ts";
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

    /** Test-only: the server (or the network) closing the socket on us. */
    remoteClose(code = 1006, reason = ""): void {
        this.readyState = 3;
        this.onclose?.({code, reason});
    }

    /** Test-only: a transport-level error, which browsers report before close. */
    fail(): void {
        this.onerror?.({type: "error"});
    }
}

function withFakeSocket<T>(fn: (latest: () => FakeSocket) => T): T {
    const g = globalThis as {WebSocket?: unknown};
    const original = g.WebSocket;
    FakeSocket.instances = [];
    g.WebSocket = FakeSocket as unknown as typeof WebSocket;

    const restore = () => {g.WebSocket = original};

    let result: T;
    try {
        result = fn(() => FakeSocket.instances[FakeSocket.instances.length - 1]);
    } catch (e) {
        restore();
        throw e;
    }

    // An async body is still running when it returns its promise, so the stub
    // has to outlive the call rather than be torn down in a `finally`.
    if (result instanceof Promise) {
        return result.finally(restore) as T;
    }

    restore();

    return result;
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

// The supervision half of the contract (plans/client-transport-failover.md,
// Step 1). A supervisor has to be able to attach and detach per channel, to
// learn that a channel died, and to learn whether one ever came up at all —
// none of which the setter-shaped original could express.

test("onMessage supports several handlers, and each unsubscribe detaches only its own", () => {
    withFakeSocket((latest) => {
        const transport = new WebSocketTransport();
        void transport.connect("ws://test");

        const first: string[] = [];
        const second: string[] = [];
        const offFirst = transport.onMessage((data) => first.push(data));
        transport.onMessage((data) => second.push(data));

        latest().onmessage?.({data: "a"});
        offFirst();
        latest().onmessage?.({data: "b"});

        assert.deepStrictEqual(first, ["a"], "the unsubscribed handler stops receiving");
        assert.deepStrictEqual(second, ["a", "b"], "the other handler is untouched");
    });
});

test("onOpen returns a working unsubscribe", () => {
    withFakeSocket((latest) => {
        const transport = new WebSocketTransport();
        void transport.connect("ws://test");

        let fired = 0;
        const off = transport.onOpen(() => {fired++});
        off();

        latest().open();

        assert.equal(fired, 0, "an unsubscribed open observer does not fire");
    });
});

test("onClose reports a remote close with a reason — the signal a supervisor fails over on", () => {
    withFakeSocket((latest) => {
        const transport = new WebSocketTransport();
        void transport.connect("ws://test");
        latest().open();

        const reasons: string[] = [];
        transport.onClose((reason) => reasons.push(reason));

        latest().remoteClose(1006, "abnormal");

        assert.equal(reasons.length, 1, "the close is reported exactly once");
        assert.match(reasons[0], /1006/, "the reason carries the close code");
    });
});

// Without this, a supervisor tearing a channel down during a switch would see
// its own close as a failure and immediately fail over again.
test("close() by the owner does NOT fire onClose", () => {
    withFakeSocket((latest) => {
        const transport = new WebSocketTransport();
        void transport.connect("ws://test");
        latest().open();

        let closes = 0;
        transport.onClose(() => {closes++});

        transport.close();

        assert.equal(closes, 0, "a deliberate teardown is not a failure");
    });
});

test("a throwing handler does not stop the next one being notified", () => {
    withFakeSocket((latest) => {
        const transport = new WebSocketTransport();
        void transport.connect("ws://test");

        let second = 0;
        transport.onMessage(() => {throw new Error("boom")});
        transport.onMessage(() => {second++});

        const warn = console.warn;
        console.warn = () => undefined;
        try {
            latest().onmessage?.({data: "a"});
        } finally {
            console.warn = warn;
        }

        assert.equal(second, 1, "the second handler still ran");
    });
});

test("connect resolves once the socket opens", async () => {
    await withFakeSocket(async (latest) => {
        const transport = new WebSocketTransport();
        const connected = transport.connect("ws://test");

        latest().open();

        await connected;
    });
});

test("connect rejects when the socket fails before it ever opens", async () => {
    await withFakeSocket(async (latest) => {
        const transport = new WebSocketTransport();
        const connected = transport.connect("ws://test");

        latest().remoteClose(1006, "refused");

        await assert.rejects(connected, /websocket/);
    });
});

// A channel that opens and dies later is a failover, not a connect failure —
// the two must not be conflated or a switch would look like a bad candidate.
test("a close after open settles nothing: the connect promise already resolved", async () => {
    await withFakeSocket(async (latest) => {
        const transport = new WebSocketTransport();
        const connected = transport.connect("ws://test");

        latest().open();
        await connected;

        latest().remoteClose(1006, "later");
    });
});

test("connectWithTimeout rejects a channel that never comes up, and closes it", async () => {
    await withFakeSocket(async (latest) => {
        const transport = new WebSocketTransport();

        await assert.rejects(connectWithTimeout(transport, "ws://test", 10), /timed out/);

        assert.equal(latest().readyState, 3, "the abandoned socket is closed, not left dangling");
    });
});

test("connectWithTimeout resolves when the channel comes up inside the budget", async () => {
    await withFakeSocket(async (latest) => {
        const transport = new WebSocketTransport();
        const connected = connectWithTimeout(transport, "ws://test", 1000);

        latest().open();

        await connected;
    });
});

test("a transport carries a name for diagnostics", () => {
    assert.equal(new WebSocketTransport().name, "websocket");
});

/** A minimal ClientTransport stand-in — no WebSocket involved at all. */
class FakeTransport implements ClientTransport {
    readonly name = "fake";
    connectedTo?: string;
    sent: object[] = [];
    private messageHandler?: (data: string) => void;

    connect(url: string): Promise<void> {
        this.connectedTo = url;
        return Promise.resolve();
    }

    send(message: object): void {
        this.sent.push(message);
    }

    onMessage(handler: (data: string) => void): () => void {
        this.messageHandler = handler;
        return () => {this.messageHandler = undefined};
    }

    onOpen(_handler: () => void): () => void {
        // Not exercised here: these tests care about send/receive wiring,
        // not the open lifecycle (covered in message-handler.node-test.ts).
        return () => undefined;
    }

    onClose(_handler: (reason: string) => void): () => void {
        return () => undefined;
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
