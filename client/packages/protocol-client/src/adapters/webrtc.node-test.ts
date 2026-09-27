import test from "node:test";
import assert from "node:assert/strict";

import {WebRtcTransportClient} from "./webrtc.ts";
import {brandCheckedFetch, record} from "../test-fakes.ts";
import type {FetchLike} from "../transport.ts";

class Emitter {
    private listeners = new Map<string, ((e: any) => void)[]>();

    addEventListener(type: string, fn: (e: any) => void) {
        this.listeners.set(type, [...(this.listeners.get(type) ?? []), fn]);
    }

    dispatch(type: string, e: any = {}) {
        for (const fn of this.listeners.get(type) ?? []) {
            fn(e);
        }
    }
}

class FakeDataChannel extends Emitter {
    binaryType = "blob";
    readyState = "connecting";
    sent: unknown[] = [];

    readonly label: string;

    constructor(label: string) {
        super();
        this.label = label;
    }

    send(d: unknown) {
        this.sent.push(d);
    }
}

const calls: string[] = [];

class FakePeerConnection extends Emitter {
    static last: FakePeerConnection;

    iceGatheringState = "new";
    connectionState = "new";
    localDescription: {type: string, sdp: string} | null = null;
    remoteDescription: unknown = null;
    channel?: FakeDataChannel;
    closed = false;

    readonly config: unknown;

    constructor(config: unknown) {
        super();
        this.config = config;
        FakePeerConnection.last = this;
    }

    createDataChannel(label: string) {
        calls.push("createDataChannel");
        this.channel = new FakeDataChannel(label);
        return this.channel;
    }

    async createOffer() {
        calls.push("createOffer");
        return {type: "offer", sdp: "v=0 offer"};
    }

    async setLocalDescription(d: {type: string, sdp: string}) {
        this.localDescription = {...d, sdp: d.sdp + " +candidates"};
        this.iceGatheringState = "gathering";
    }

    async setRemoteDescription(d: unknown) {
        this.remoteDescription = d;
    }

    finishGathering() {
        this.iceGatheringState = "complete";
        this.dispatch("icegatheringstatechange");
    }

    close() {
        this.closed = true;
    }
}

const tick = () => new Promise((r) => setTimeout(r, 0));

function setup(status = 200) {
    calls.length = 0;
    const posts: {url: string, body: unknown}[] = [];
    // Brand-checked like a browser's, so calling it as a method fails.
    const fetch: FetchLike = brandCheckedFetch(async (url: string, init?: Parameters<FetchLike>[1]) => {
        posts.push({url, body: JSON.parse(String(init?.body))});
        return {ok: status === 200, status, body: null, json: async () => ({type: "answer", sdp: "v=0 answer"})};
    });
    const c = new WebRtcTransportClient({
        url: "http://server:5002",
        fetch,
        RTCPeerConnection: FakePeerConnection,
        iceServers: [{urls: "stun:stun.example:3478"}],
    });
    return {c, posts, log: record(c)};
}

test("creates the data channel before the offer, waits for gathering, then posts the offer", async () => {
    const {c, posts} = setup();
    const opening = c.connect();
    await tick();

    const pc = FakePeerConnection.last;
    assert.deepEqual(calls, ["createDataChannel", "createOffer"]);
    assert.deepEqual(pc.config, {iceServers: [{urls: "stun:stun.example:3478"}]});
    assert.equal(posts.length, 0, "must not post before gathering completes");

    pc.finishGathering();
    await tick();

    assert.deepEqual(posts, [{url: "http://server:5002/webrtc/offer", body: {type: "offer", sdp: "v=0 offer +candidates"}}]);
    assert.deepEqual(pc.remoteDescription, {type: "answer", sdp: "v=0 answer"});

    pc.channel!.readyState = "open";
    pc.channel!.dispatch("open");
    await opening;
});

async function connected() {
    const s = setup();
    const opening = s.c.connect();
    await tick();
    const pc = FakePeerConnection.last;
    pc.finishGathering();
    await tick();
    pc.channel!.dispatch("open");
    await opening;
    return {...s, pc, dc: pc.channel!};
}

test("a query on the base URL (e.g. ?debug=1) goes on the offer", async () => {
    calls.length = 0;
    const urls: string[] = [];
    const c = new WebRtcTransportClient({
        url: "http://server:5002?debug=1",
        fetch: async (url) => { urls.push(url); return {ok: false, status: 503, body: null, json: async () => ({})}; },
        RTCPeerConnection: FakePeerConnection,
    });
    const opening = c.connect();
    await tick();
    FakePeerConnection.last.finishGathering();
    await assert.rejects(opening);

    assert.deepEqual(urls, ["http://server:5002/webrtc/offer?debug=1"]);
});

test("messages arrive as text or bytes, and sends go over the channel", async () => {
    const {c, log, dc} = await connected();

    assert.equal(dc.binaryType, "arraybuffer");

    dc.dispatch("message", {data: '{"a":1}'});
    dc.dispatch("message", {data: new Uint8Array([7, 8]).buffer});
    assert.deepEqual(log.messages, ['{"a":1}', new Uint8Array([7, 8])]);

    c.send("{}");
    c.send(new Uint8Array([9]));
    assert.deepEqual(dc.sent, ["{}", new Uint8Array([9])]);
});

test("a refused offer rejects connect", async () => {
    const {c} = setup(503);
    const opening = c.connect();
    await tick();
    FakePeerConnection.last.finishGathering();

    await assert.rejects(opening, /503/);
    assert.ok(FakePeerConnection.last.closed);
});

test("a failed peer connection ends the connection once", async () => {
    const {log, pc} = await connected();

    pc.connectionState = "failed";
    pc.dispatch("connectionstatechange");
    pc.channel!.dispatch("close");

    assert.equal(log.closes.length, 1);
    assert.ok(pc.closed);
});

test("close() closes the peer connection without firing onClose", async () => {
    const {c, log, pc} = await connected();

    c.close();
    pc.channel!.dispatch("close");

    assert.ok(pc.closed);
    assert.equal(log.closes.length, 0);
});

test("without an RTCPeerConnection the error says how to get one", () => {
    assert.throws(
        () => new WebRtcTransportClient({url: "http://x", fetch: async () => { throw new Error(); }}),
        /react-native-webrtc/,
    );
});
