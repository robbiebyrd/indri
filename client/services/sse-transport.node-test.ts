// The third channel (plans/client-transport-failover.md, Step 3). Only the
// PURE parts are covered here: the action-to-route mapping, and the rejections
// that keep a channel which cannot possibly work out of the candidate list.
// The stream itself needs an EventSource, which `node --experimental-strip-types`
// does not provide — same constraint that put the WebRTC helpers in
// webrtc-signal.ts.
import test from "node:test";
import assert from "node:assert/strict";

import {SseRestTransport, restRequest} from "./sse-transport.ts";

import type {EventStream, StreamFactory} from "./sse-stream.ts";

const API = "http://localhost:8080/api";

test("restRequest routes an action to its REST path and strips the action from the body", () => {
    assert.deepStrictEqual(
        restRequest(API, {action: "login", email: "a@b.c", password: "pw"}),
        {url: "http://localhost:8080/api/login", body: {email: "a@b.c", password: "pw"}},
    );
});

test("restRequest handles an action with no arguments", () => {
    assert.deepStrictEqual(
        restRequest(API, {action: "leave"}),
        {url: "http://localhost:8080/api/leave", body: {}},
    );
});

// internal/transport/rest/routes.go renames this on the way in: the reconnect
// handler reads "sessionId", but the REST route takes the friendlier "token".
// Every other transport sends "sessionId", so the rename has to happen here or
// reconnect fails with a 400 and the player is silently logged out on failover
// — exactly the failure this whole plan exists to prevent.
test("restRequest renames reconnect's sessionId to the token field the REST route wants", () => {
    assert.deepStrictEqual(
        restRequest(API, {action: "reconnect", sessionId: "abc123"}),
        {url: "http://localhost:8080/api/reconnect", body: {token: "abc123"}},
    );
});

test("restRequest rejects a message with no action rather than POSTing to /api/undefined", () => {
    assert.equal(restRequest(API, {}), undefined);
    assert.equal(restRequest(API, {action: 42}), undefined);
    assert.equal(restRequest(API, {action: ""}), undefined);
});

// A path separator in the action would otherwise let a caller reach any route
// on the origin, and "../" would climb out of /api entirely.
test("restRequest rejects an action that is not a plain route name", () => {
    assert.equal(restRequest(API, {action: "../rtc/offer"}), undefined);
    assert.equal(restRequest(API, {action: "a/b"}), undefined);
});

test("connect rejects when no session token is available", async () => {
    const transport = new SseRestTransport(() => null);

    await assert.rejects(transport.connect("ws://localhost:8080/ws"), /token/);
});

test("connect rejects on an empty token, which the stream would reject anyway", async () => {
    const transport = new SseRestTransport(() => "");

    await assert.rejects(transport.connect("ws://localhost:8080/ws"), /token/);
});

// The stream is now INJECTED rather than constructed from a global, which is
// what lets the lifecycle below be tested at all — `node
// --experimental-strip-types` has no EventSource, and react-native-sse cannot
// be loaded here either (it resolves through Metro).
class FakeStream implements EventStream {
    static last?: FakeStream;

    readonly url: string;
    readonly token: string;
    closes = 0;

    private openHandler?: () => void;
    private messageHandler?: (data: string) => void;
    private errorHandler?: (reason: string) => void;

    constructor(url: string, token: string) {
        this.url = url;
        this.token = token;
        FakeStream.last = this;
    }

    onOpen(handler: () => void): void {this.openHandler = handler}
    onMessage(handler: (data: string) => void): void {this.messageHandler = handler}
    onError(handler: (reason: string) => void): void {this.errorHandler = handler}

    close(): void {
        this.closes++;
        // A real stream stops calling back once closed; the fake must too, or
        // it cannot show that a deliberate close fires nothing.
        this.openHandler = undefined;
        this.messageHandler = undefined;
        this.errorHandler = undefined;
    }

    /** Test-only. */
    open(): void {this.openHandler?.()}
    deliver(data: string): void {this.messageHandler?.(data)}
    fail(reason = "stream error"): void {this.errorHandler?.(reason)}
}

const fakeStreams: StreamFactory = (url, token) => new FakeStream(url, token);

function latest(): FakeStream {
    assert.ok(FakeStream.last, "no stream was opened");
    return FakeStream.last;
}

test("connect resolves once the stream opens, and forwards inbound frames", async () => {
    const transport = new SseRestTransport(() => "abc123", fakeStreams);
    const connected = transport.connect("ws://localhost:8080/ws");

    const received: string[] = [];
    transport.onMessage((data) => received.push(data));

    latest().open();
    await connected;

    latest().deliver('{"op":"update"}');
    assert.deepStrictEqual(received, ['{"op":"update"}']);
});

test("connect rejects when the stream fails before it opens", async () => {
    const transport = new SseRestTransport(() => "abc123", fakeStreams);

    const connected = transport.connect("ws://localhost:8080/ws");
    latest().fail("401");

    await assert.rejects(connected, /sse-rest/);
});

// The library reconnects on its own every 5s by default. That is a SECOND
// backoff competing with the supervisor's, so a failure has to close the
// stream and hand the decision upward.
test("a failure after open closes the stream and reports it through onClose", async () => {
    const transport = new SseRestTransport(() => "abc123", fakeStreams);
    const connected = transport.connect("ws://localhost:8080/ws");

    const reasons: string[] = [];
    transport.onClose((reason) => reasons.push(reason));

    latest().open();
    await connected;

    const stream = latest();
    stream.fail("dropped");

    assert.equal(stream.closes, 1, "the stream is closed rather than left to reconnect itself");
    assert.equal(reasons.length, 1, "and the supervisor is told exactly once");
});

test("close() by the owner does NOT fire onClose", async () => {
    const transport = new SseRestTransport(() => "abc123", fakeStreams);
    const connected = transport.connect("ws://localhost:8080/ws");

    let closes = 0;
    transport.onClose(() => {closes++});

    latest().open();
    await connected;

    transport.close();

    assert.equal(closes, 0, "a deliberate teardown is not a failure");
    assert.equal(latest().closes, 1, "the stream is released");
});

test("close() settles a connect that never opened, rather than leaving it pending", async () => {
    const transport = new SseRestTransport(() => "abc123", fakeStreams);

    const connected = transport.connect("ws://localhost:8080/ws");
    transport.close();

    await assert.rejects(connected, /closed/);
});

// The stream factory receives the token SEPARATELY from the url. That is what
// lets the native implementation put it in an Authorization header while the
// web one appends it as a query parameter — the server accepts either
// (sse.go's tokenFrom prefers the header).
test("the stream is handed the events url and the token as separate arguments", async () => {
    const transport = new SseRestTransport(() => "abc123", fakeStreams);
    const connected = transport.connect("ws://localhost:8080/ws");

    assert.equal(latest().url, "http://localhost:8080/events", "no token is baked into the url");
    assert.equal(latest().token, "abc123");

    transport.close();
    await connected.catch(() => undefined);
});

// react-native has no EventSource, and on that platform the default factory is
// react-native-sse instead. This asserts the fallback that remains when NO
// implementation is available at all.
test("connect rejects when no stream implementation is available", async () => {
    const g = globalThis as {EventSource?: unknown};
    const original = g.EventSource;
    delete g.EventSource;

    try {
        const transport = new SseRestTransport(() => "abc123");
        await assert.rejects(transport.connect("ws://localhost:8080/ws"), /event stream/);
    } finally {
        if (original !== undefined) {
            g.EventSource = original;
        }
    }
});

// On web the stream URL carries the bearer token as a query parameter (browser
// EventSource cannot set headers), so it must never reach a log or an error
// message — both of which routinely end up in a bug report.
test("no rejection carries the session token", async () => {
    const g = globalThis as {EventSource?: unknown};
    const original = g.EventSource;
    delete g.EventSource;

    try {
        const transport = new SseRestTransport(() => "s3cr3t-token");
        await transport.connect("ws://localhost:8080/ws").then(
            () => assert.fail("expected a rejection"),
            (err: unknown) => {
                assert.doesNotMatch(String(err), /s3cr3t-token/, "the token must not appear in an error");
            },
        );
    } finally {
        if (original !== undefined) {
            g.EventSource = original;
        }
    }
});

test("the transport carries a name for diagnostics", () => {
    assert.equal(new SseRestTransport(() => null).name, "sse-rest");
});
