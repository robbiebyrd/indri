// The third channel (plans/client-transport-failover.md, Step 3). Only the
// PURE parts are covered here: the action-to-route mapping, and the rejections
// that keep a channel which cannot possibly work out of the candidate list.
// The stream itself needs an EventSource, which `node --experimental-strip-types`
// does not provide — same constraint that put the WebRTC helpers in
// webrtc-signal.ts.
import test from "node:test";
import assert from "node:assert/strict";

import {SseRestTransport, restRequest} from "./sse-transport.ts";

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

// react-native has no EventSource. Rejecting is what keeps this channel out of
// the rotation on a platform that cannot run it, instead of throwing a
// ReferenceError up through the supervisor.
test("connect rejects when the platform has no EventSource", async () => {
    const g = globalThis as {EventSource?: unknown};
    const original = g.EventSource;
    delete g.EventSource;

    try {
        const transport = new SseRestTransport(() => "abc123");
        await assert.rejects(transport.connect("ws://localhost:8080/ws"), /EventSource/);
    } finally {
        if (original !== undefined) {
            g.EventSource = original;
        }
    }
});

// The stream URL carries the bearer token as a query parameter (EventSource
// cannot set headers), so it must never reach a log or an error message — both
// of which routinely end up in a bug report.
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
