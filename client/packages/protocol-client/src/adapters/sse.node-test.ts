import test from "node:test";
import assert from "node:assert/strict";

import {SseTransportClient} from "./sse.ts";
import {brandCheckedFetch, captureWarnings, record} from "../test-fakes.ts";
import type {FetchLike} from "../transport.ts";

type Call = {url: string, init?: Parameters<FetchLike>[1]};

/** A fake server: one controllable event stream plus scripted POST replies. */
function fakeServer() {
    const calls: Call[] = [];
    let stream!: ReadableStreamDefaultController<Uint8Array>;
    let streamStatus = 200;
    const postReplies: {status: number, gate?: Promise<void>}[] = [];

    const fetch: FetchLike = async (url, init) => {
        calls.push({url, init});

        if (url.split("?")[0].endsWith("/sse/stream")) {
            const body = new ReadableStream<Uint8Array>({start: (c) => { stream = c; }});
            init?.signal?.addEventListener("abort", () => stream.error(new Error("aborted")));
            return {ok: streamStatus === 200, status: streamStatus, body, json: async () => ({})};
        }

        const reply = postReplies.shift() ?? {status: 204};
        await reply.gate;
        return {ok: reply.status < 300, status: reply.status, body: null, json: async () => ({})};
    };

    return {
        fetch,
        calls,
        posts: () => calls.filter((c) => c.init?.method === "POST"),
        emit: (text: string) => stream.enqueue(new TextEncoder().encode(text)),
        end: () => stream.close(),
        refuseStream: (status: number) => { streamStatus = status; },
        replyToNextPost: (status: number, gate?: Promise<void>) => postReplies.push({status, gate}),
    };
}

const tick = () => new Promise((r) => setTimeout(r, 0));

async function connected() {
    const server = fakeServer();
    const c = new SseTransportClient({url: "http://server:5002/", fetch: server.fetch});
    const log = record(c);
    const opening = c.connect();
    await tick();
    server.emit("event: connected\ndata: conn-id\n\n");
    await opening;
    return {server, c, log};
}

test("calls fetch unbound, so a browser's brand-checked fetch works", async () => {
    const server = fakeServer();
    const c = new SseTransportClient({url: "http://server", fetch: brandCheckedFetch(server.fetch)});
    const opening = c.connect();
    await tick();
    server.emit("event: connected\ndata: id\n\n");
    await opening;

    c.send("{}");
    await tick();
    assert.equal(server.posts().length, 1);
});

test("a query on the base URL (e.g. ?debug=1) goes on the stream request only", async () => {
    const server = fakeServer();
    const c = new SseTransportClient({url: "http://server:5002/?debug=1", fetch: server.fetch});
    const opening = c.connect();
    await tick();
    server.emit("event: connected\ndata: id\n\n");
    await opening;

    c.send("{}");
    await tick();

    assert.equal(server.calls[0].url, "http://server:5002/sse/stream?debug=1");
    assert.equal(server.posts()[0].url, "http://server:5002/sse/send");
});

test("opens the stream and resolves connect on the connected event", async () => {
    const {server, log} = await connected();

    assert.equal(server.calls[0].url, "http://server:5002/sse/stream");
    assert.equal(server.calls[0].init?.headers?.Accept, "text/event-stream");
    assert.equal(log.opens, 1);
});

test("a message in the same chunk as the connected event is not lost", async () => {
    const server = fakeServer();
    const c = new SseTransportClient({url: "http://server", fetch: server.fetch});
    const log = record(c);
    const opening = c.connect();
    await tick();

    server.emit('event: connected\ndata: id\n\ndata: {"stage":{"currentScene":"login"}}\n\n');
    await opening;
    await tick();

    assert.deepEqual(log.messages, ['{"stage":{"currentScene":"login"}}']);
});

test("text events are strings and binary events are decoded bytes", async () => {
    const {server, log} = await connected();

    server.emit('data: {"a":1}\n\nevent: binary\ndata: AIGh/wrA\n\n');
    await tick();

    assert.deepEqual(log.messages, ['{"a":1}', new Uint8Array([0x00, 0x81, 0xa1, 0xff, 0x0a, 0xc0])]);
});

test("sends POST with the connection id, one at a time, in order", async () => {
    const {server, c} = await connected();

    let releaseFirst!: () => void;
    server.replyToNextPost(204, new Promise((r) => { releaseFirst = r; }));

    c.send('{"action":"login"}');
    c.send('{"action":"join"}');
    await tick();

    assert.equal(server.posts().length, 1, "second send must wait for the first to be handled");

    releaseFirst();
    await tick();
    await tick();

    const posts = server.posts();
    assert.equal(posts.length, 2);
    assert.deepEqual(posts.map((p) => p.init?.body), ['{"action":"login"}', '{"action":"join"}']);
    assert.equal(posts[0].url, "http://server:5002/sse/send");
    assert.equal(posts[0].init?.headers?.["X-Indri-Connection-Id"], "conn-id");
});

test("binary sends go as octet-stream", async () => {
    const {server, c} = await connected();

    c.send(new Uint8Array([1, 2, 3]));
    await tick();

    const post = server.posts()[0];
    assert.equal(post.init?.headers?.["Content-Type"], "application/octet-stream");
    assert.deepEqual(post.init?.body, new Uint8Array([1, 2, 3]));
});

test("a 404 on send means the server forgot us: onClose fires and the stream stops", async () => {
    const {server, c, log} = await connected();

    server.replyToNextPost(404);
    c.send("{}");
    await tick();
    await tick();

    assert.equal(log.closes.length, 1);
    assert.equal(server.calls[0].init?.signal?.aborted, true);
});

test("the stream ending fires onClose once", async () => {
    const {server, log} = await connected();

    server.end();
    await tick();
    await tick();

    assert.equal(log.closes.length, 1);
});

test("a refused stream rejects connect", async () => {
    const server = fakeServer();
    server.refuseStream(403);
    const c = new SseTransportClient({url: "http://server", fetch: server.fetch});

    await assert.rejects(c.connect(), /403/);
});

test("close() aborts the stream without firing onClose", async () => {
    const {server, c, log} = await connected();

    c.close();
    await tick();

    assert.equal(server.calls[0].init?.signal?.aborted, true);
    assert.equal(log.closes.length, 0);
});

test("sends before the connected event are dropped with a warning", async () => {
    const server = fakeServer();
    const c = new SseTransportClient({url: "http://server", fetch: server.fetch});
    void c.connect().catch(() => {});
    await tick();

    const warnings = await captureWarnings(() => c.send("{}"));

    assert.equal(server.posts().length, 0);
    assert.equal(warnings.length, 1);
    c.close();
});
