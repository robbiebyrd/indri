import test from "node:test";
import assert from "node:assert/strict";

import {createTransportClient, TRANSPORT_KINDS} from "./factory.ts";
import {GraphQLWsTransportClient} from "./adapters/graphqlws.ts";
import {SseTransportClient} from "./adapters/sse.ts";
import {WebRtcTransportClient} from "./adapters/webrtc.ts";
import {WebSocketTransportClient} from "./adapters/websocket.ts";
import {FakeWebSocket} from "./test-fakes.ts";

const deps = {
    WebSocket: FakeWebSocket,
    fetch: async () => { throw new Error("unused"); },
    RTCPeerConnection: class {} as any,
};

test("each kind builds its adapter; kind names match the server's INDRI_TRANSPORTS values", () => {
    const expected = {
        ws: WebSocketTransportClient,
        sse: SseTransportClient,
        graphqlws: GraphQLWsTransportClient,
        webrtc: WebRtcTransportClient,
    };

    assert.deepEqual([...TRANSPORT_KINDS].sort(), Object.keys(expected).sort());

    for (const [kind, cls] of Object.entries(expected)) {
        assert.ok(createTransportClient(kind, {url: "x", ...deps}) instanceof cls, kind);
    }
});

test("an unknown kind is a clear error naming the valid ones", () => {
    assert.throws(() => createTransportClient("carrier-pigeon", {url: "x", ...deps}), /ws, sse, graphqlws, webrtc/);
});
