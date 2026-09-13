// endpoints.ts is the single derivation of every server route from the one
// configured base URL (plans/client-transport-failover.md, Step 2). With three
// channels, a per-transport copy of this logic would eventually disagree about
// the scheme or the path — and the failure mode is a channel that silently
// never connects, which the supervisor would read as "this transport is
// unavailable here" rather than "the URL is wrong".
import test from "node:test";
import assert from "node:assert/strict";

import {endpoints} from "./endpoints.ts";

test("a ws:// base yields every channel's endpoint on the same origin", () => {
    const e = endpoints("ws://localhost:8080/ws");

    assert.deepStrictEqual(e, {
        websocket: "ws://localhost:8080/ws",
        rtcOffer: "http://localhost:8080/rtc/offer",
        events: "http://localhost:8080/events",
        api: "http://localhost:8080/api",
    });
});

// Getting this wrong downgrades a secure deployment to cleartext, or — under a
// browser's mixed-content rules — blocks the request outright.
test("a wss:// base maps to https://, never http://", () => {
    const e = endpoints("wss://play.example.com/ws");

    assert.equal(e.websocket, "wss://play.example.com/ws");
    assert.equal(e.rtcOffer, "https://play.example.com/rtc/offer");
    assert.equal(e.events, "https://play.example.com/events");
    assert.equal(e.api, "https://play.example.com/api");
});

// The other three routes are absolute on the origin, so whatever path the
// configured base carries is irrelevant to them.
test("a base without the /ws suffix still produces correct endpoints", () => {
    const e = endpoints("ws://localhost:8080");

    assert.equal(e.websocket, "ws://localhost:8080", "the websocket url is used as configured");
    assert.equal(e.rtcOffer, "http://localhost:8080/rtc/offer");
    assert.equal(e.events, "http://localhost:8080/events");
    assert.equal(e.api, "http://localhost:8080/api");
});

test("a query string or fragment on the base does not leak into the derived routes", () => {
    const e = endpoints("ws://localhost:8080/ws?debug=1#frag");

    assert.equal(e.rtcOffer, "http://localhost:8080/rtc/offer");
    assert.equal(e.events, "http://localhost:8080/events");
});

// The server mounts every route absolutely on the origin (ws.go's "/ws",
// webrtc.go's "/rtc/offer", sse.go's "/events", rest.go's "/api/"), so the
// base's own path tells us nothing about where the others live.
test("the base's path does not relocate the other routes", () => {
    const e = endpoints("wss://example.com/indri/ws");

    assert.equal(e.rtcOffer, "https://example.com/rtc/offer");
    assert.equal(e.events, "https://example.com/events");
    assert.equal(e.api, "https://example.com/api");
});

test("an unparseable base throws rather than silently dialling the page origin", () => {
    assert.throws(() => endpoints(""), /EXPO_PUBLIC_API_URL/);
    assert.throws(() => endpoints("not a url"), /EXPO_PUBLIC_API_URL/);
});
