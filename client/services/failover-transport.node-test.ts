// The supervisor (plans/client-transport-failover.md, Step 4). Its logic is
// pure policy — pick, fall through, switch, queue — so it is driven here
// against fake channels, which is the only way it could be tested at all: the
// client's runner (`node --experimental-strip-types`) has neither WebSocket
// nor WebRTC nor EventSource.
import test from "node:test";
import assert from "node:assert/strict";

import {FailoverTransport} from "./failover-transport.ts";

import type {ClientTransport} from "./transport.ts";

/** How a fake channel behaves when asked to connect. */
type Mode = "open" | "reject" | "hang" | "manual";

/** A ClientTransport whose whole lifecycle is driven by the test. */
class FakeChannel implements ClientTransport {
    connects = 0;
    closes = 0;
    sent: object[] = [];

    private pending?: {resolve: () => void, reject: (err: Error) => void};
    private readonly messageHandlers = new Set<(data: string) => void>();
    private readonly openHandlers = new Set<() => void>();
    private readonly closeHandlers = new Set<(reason: string) => void>();

    readonly name: string;
    private readonly mode: Mode;

    // Written out rather than as parameter properties: `node
    // --experimental-strip-types` is strip-only and rejects that syntax.
    constructor(name: string, mode: Mode = "open") {
        this.name = name;
        this.mode = mode;
    }

    connect(_url: string): Promise<void> {
        this.connects++;

        const connected = new Promise<void>((resolve, reject) => {
            this.pending = {resolve, reject};
        });

        if (this.mode === "open") {
            queueMicrotask(() => this.comeUp());
        }
        if (this.mode === "reject") {
            queueMicrotask(() => this.refuse("refused"));
        }
        // "hang" and "manual" never settle themselves.

        return connected;
    }

    send(message: object): void {
        this.sent.push(message);
    }

    onMessage(handler: (data: string) => void): () => void {
        this.messageHandlers.add(handler);
        return () => {this.messageHandlers.delete(handler)};
    }

    onOpen(handler: () => void): () => void {
        this.openHandlers.add(handler);
        return () => {this.openHandlers.delete(handler)};
    }

    onClose(handler: (reason: string) => void): () => void {
        this.closeHandlers.add(handler);
        return () => {this.closeHandlers.delete(handler)};
    }

    close(): void {
        this.closes++;
        this.refuse("closed before the connection opened");
    }

    /** Test-only: the channel comes up. */
    comeUp(): void {
        const pending = this.pending;
        if (!pending) {
            return;
        }
        this.pending = undefined;
        pending.resolve();
        for (const handler of [...this.openHandlers]) {
            handler();
        }
    }

    /** Test-only: the channel never comes up. */
    refuse(reason: string): void {
        const pending = this.pending;
        if (!pending) {
            return;
        }
        this.pending = undefined;
        pending.reject(new Error(reason));
    }

    /** Test-only: an established channel dies. */
    die(reason = "gone"): void {
        for (const handler of [...this.closeHandlers]) {
            handler(reason);
        }
    }

    /** Test-only: an inbound frame arrives. */
    deliver(data: string): void {
        for (const handler of [...this.messageHandlers]) {
            handler(data);
        }
    }
}

const OPTS = {attemptTimeoutMs: 40, backoffMs: 20, queueLimit: 4};

async function waitFor(predicate: () => boolean, label: string, timeoutMs = 2000): Promise<void> {
    const deadline = Date.now() + timeoutMs;
    while (Date.now() < deadline) {
        if (predicate()) {
            return;
        }
        await new Promise((resolve) => setTimeout(resolve, 1));
    }
    assert.fail(`timed out waiting for ${label}`);
}

/** Silences the warnings the drop paths deliberately emit. */
async function quietly<T>(fn: () => Promise<T>): Promise<T> {
    const warn = console.warn;
    console.warn = () => undefined;
    try {
        return await fn();
    } finally {
        console.warn = warn;
    }
}

test("picks the first candidate that connects, in preference order", async () => {
    const first = new FakeChannel("first");
    const second = new FakeChannel("second");
    const failover = new FailoverTransport(() => [first, second], OPTS);

    await failover.connect("ws://test");

    assert.equal(first.connects, 1, "the preferred candidate is tried");
    assert.equal(second.connects, 0, "a working preferred channel means the rest are never dialled");

    failover.close();
});

test("falls through to the next candidate when one rejects", async () => {
    const dead = new FakeChannel("dead", "reject");
    const alive = new FakeChannel("alive");
    const failover = new FailoverTransport(() => [dead, alive], OPTS);

    await failover.connect("ws://test");

    assert.equal(dead.connects, 1);
    assert.equal(alive.connects, 1, "the next candidate is tried");
    assert.ok(dead.closes >= 1, "the rejected candidate is released, not left dangling");

    failover.close();
});

// A channel that neither opens nor errors is the dangerous case: without a
// budget it stalls startup behind a TCP timeout with nothing to report.
test("falls through when a candidate hangs instead of failing", async () => {
    const hung = new FakeChannel("hung", "hang");
    const alive = new FakeChannel("alive");
    const failover = new FailoverTransport(() => [hung, alive], OPTS);

    await failover.connect("ws://test");

    assert.equal(alive.connects, 1, "the hung candidate did not stall the cycle");
    assert.ok(hung.closes >= 1, "the hung candidate is closed so it cannot come up later");

    failover.close();
});

test("switches to the next candidate when the active channel closes", async () => {
    const first = new FakeChannel("first");
    const second = new FakeChannel("second");
    const failover = new FailoverTransport(() => [first, second], OPTS);

    await failover.connect("ws://test");
    first.die();

    await waitFor(() => second.connects === 1, "the second channel to be dialled");

    failover.send({action: "ping"});
    assert.deepStrictEqual(second.sent, [{action: "ping"}], "sends go to the new channel");
    assert.deepStrictEqual(first.sent, [], "and never to the dead one");

    failover.close();
});

test("emits one onOpen per successful connect, including after a switch", async () => {
    const first = new FakeChannel("first");
    const second = new FakeChannel("second");
    const failover = new FailoverTransport(() => [first, second], OPTS);

    let opens = 0;
    failover.onOpen(() => {opens++});

    await failover.connect("ws://test");
    assert.equal(opens, 1, "one open for the first channel");

    first.die();
    await waitFor(() => opens === 2, "a second open after the switch");

    failover.close();
});

test("forwards inbound frames from the active channel, and stops when it is dropped", async () => {
    const first = new FakeChannel("first");
    const second = new FakeChannel("second");
    const failover = new FailoverTransport(() => [first, second], OPTS);

    const received: string[] = [];
    failover.onMessage((data) => received.push(data));

    await failover.connect("ws://test");
    first.deliver("a");

    first.die();
    await waitFor(() => second.connects === 1, "the switch");

    // A dead channel that somehow still emits must not reach the app: its
    // frames belong to a server-side connection that no longer exists.
    first.deliver("stale");
    second.deliver("b");

    assert.deepStrictEqual(received, ["a", "b"]);

    failover.close();
});

test("a send issued before any channel is open is queued and flushed on connect", async () => {
    const channel = new FakeChannel("only", "manual");
    const failover = new FailoverTransport(() => [channel], OPTS);

    const connected = failover.connect("ws://test");

    failover.send({action: "login", email: "a@b.c"});
    assert.deepStrictEqual(channel.sent, [], "nothing is written before the channel is up");

    channel.comeUp();
    await connected;

    assert.deepStrictEqual(channel.sent, [{action: "login", email: "a@b.c"}], "the queue is flushed on open");

    failover.close();
});

// The server has no idempotency keys, so replaying a queued `create` fails and
// a replayed game action double-applies. A visibly dropped action beats an
// invisibly duplicated one.
test("a queued send is DROPPED, not replayed, when the channel changes", async () => {
    const first = new FakeChannel("first", "manual");
    const second = new FakeChannel("second");
    const failover = new FailoverTransport(() => [first, second], OPTS);

    // close() rejects this, which is the documented contract; the test is not
    // about that rejection.
    failover.connect("ws://test").catch(() => undefined);

    await quietly(async () => {
        failover.send({action: "create", code: "abcd"});
        first.refuse("gave up");

        await waitFor(() => second.connects === 1, "the fall-through");
    });

    assert.deepStrictEqual(second.sent, [], "the queued action is not replayed onto the new channel");
    assert.deepStrictEqual(first.sent, [], "and was never written to the old one either");

    failover.close();
});

test("the queue is bounded and drops the OLDEST, matching the server's own policy", async () => {
    const channel = new FakeChannel("only", "manual");
    const failover = new FailoverTransport(() => [channel], {...OPTS, queueLimit: 2});

    const connected = failover.connect("ws://test");

    await quietly(async () => {
        failover.send({n: 1});
        failover.send({n: 2});
        failover.send({n: 3});

        channel.comeUp();
        await connected;
    });

    assert.deepStrictEqual(channel.sent, [{n: 2}, {n: 3}], "the oldest was dropped, the newest kept");

    failover.close();
});

test("close stops all probing and does not resurrect a channel", async () => {
    const first = new FakeChannel("first");
    const second = new FakeChannel("second");
    const failover = new FailoverTransport(() => [first, second], OPTS);

    await failover.connect("ws://test");
    failover.close();

    assert.ok(first.closes >= 1, "the active channel is released");

    first.die();
    await new Promise((resolve) => setTimeout(resolve, OPTS.backoffMs * 3));

    assert.equal(second.connects, 0, "nothing is dialled after close");
});

test("close rejects a connect that never found a channel, rather than leaving it pending", async () => {
    const channel = new FakeChannel("only", "hang");
    const failover = new FailoverTransport(() => [channel], OPTS);

    const connected = failover.connect("ws://test");
    failover.close();

    await assert.rejects(connected, /closed/);
});

// Without a backoff, a server outage turns every client into a hot loop across
// three channels — the client-side half of a thundering herd.
test("a fully failed cycle waits for the backoff before trying again", async () => {
    const channel = new FakeChannel("only", "reject");
    const failover = new FailoverTransport(() => [channel], {...OPTS, backoffMs: 120});

    // close() rejects this, which is the documented contract; the test is not
    // about that rejection.
    failover.connect("ws://test").catch(() => undefined);

    await waitFor(() => channel.connects === 1, "the first attempt");
    await new Promise((resolve) => setTimeout(resolve, 40));

    assert.equal(channel.connects, 1, "no retry inside the backoff window");

    await waitFor(() => channel.connects === 2, "a retry after the backoff", 1000);

    failover.close();
});

// SSE cannot be a candidate until a session exists (it authenticates at the
// handshake), so the list has to be recomputed rather than captured once.
test("the candidate list is re-evaluated per cycle, so a late channel joins the rotation", async () => {
    const early = new FakeChannel("early", "reject");
    const late = new FakeChannel("late");
    let tokenExists = false;

    const failover = new FailoverTransport(
        () => (tokenExists ? [early, late] : [early]),
        {...OPTS, backoffMs: 10},
    );

    // close() rejects this, which is the documented contract; the test is not
    // about that rejection.
    failover.connect("ws://test").catch(() => undefined);

    await waitFor(() => early.connects >= 1, "the first cycle");
    assert.equal(late.connects, 0, "the ineligible candidate is not offered yet");

    tokenExists = true;

    await waitFor(() => late.connects === 1, "the newly eligible candidate to be dialled");

    failover.close();
});

test("an empty candidate list backs off instead of spinning", async () => {
    let list: ClientTransport[] = [];
    const failover = new FailoverTransport(() => list, {...OPTS, backoffMs: 10});

    // close() rejects this, which is the documented contract; the test is not
    // about that rejection.
    failover.connect("ws://test").catch(() => undefined);
    await new Promise((resolve) => setTimeout(resolve, 30));

    const channel = new FakeChannel("late");
    list = [channel];

    await waitFor(() => channel.connects === 1, "the candidate that appeared later");

    failover.close();
});

test("losing the active channel is reported through onClose", async () => {
    const first = new FakeChannel("first");
    const second = new FakeChannel("second");
    const failover = new FailoverTransport(() => [first, second], OPTS);

    const reasons: string[] = [];
    failover.onClose((reason) => reasons.push(reason));

    await failover.connect("ws://test");
    first.die("network gone");

    await waitFor(() => reasons.length === 1, "the close to be reported");
    assert.match(reasons[0], /network gone/);

    failover.close();
});

test("the supervisor is itself a ClientTransport, which is the whole design", () => {
    const failover: ClientTransport = new FailoverTransport(() => [], OPTS);

    assert.equal(failover.name, "failover");
});
