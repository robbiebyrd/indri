import test from "node:test";
import assert from "node:assert/strict";

import {GraphQLWsTransportClient} from "./graphqlws.ts";
import {FakeWebSocket, record} from "../test-fakes.ts";

async function connected() {
    const c = new GraphQLWsTransportClient({url: "ws://server/graphql", WebSocket: FakeWebSocket});
    const log = record(c);
    const opening = c.connect();
    const ws = FakeWebSocket.last();
    ws.serverOpen();
    ws.serverSend(JSON.stringify({type: "connection_ack"}));
    await opening;
    return {c, ws, log};
}

const next = (id: string, event: object) =>
    JSON.stringify({id, type: "next", payload: {data: {indriEvents: event}}});

test("handshake: subprotocol, connection_init, then the IndriEvents subscription", async () => {
    const {ws, log} = await connected();

    assert.equal(ws.url, "ws://server/graphql");
    assert.equal(ws.protocols, "graphql-transport-ws");

    const [init, subscribe] = ws.sentJSON();
    assert.deepEqual(init, {type: "connection_init"});
    assert.equal(subscribe.type, "subscribe");
    assert.equal(subscribe.payload.operationName, "IndriEvents");
    assert.equal(log.opens, 1);
});

test("next frames on the events subscription become messages, text or binary", async () => {
    const {ws, log} = await connected();
    const eventsID = ws.sentJSON()[1].id;

    ws.serverSend(next(eventsID, {text: '{"stage":{"currentScene":"login"}}'}));
    ws.serverSend(next(eventsID, {b64: "AIGh/wrA"}));
    ws.serverSend(next("some-other-op", {text: "ignored"}));

    assert.deepEqual(log.messages, [
        '{"stage":{"currentScene":"login"}}',
        new Uint8Array([0x00, 0x81, 0xa1, 0xff, 0x0a, 0xc0]),
    ]);
});

test("each send is its own Send operation with a fresh id", async () => {
    const {c, ws} = await connected();

    c.send('{"action":"refresh"}');
    c.send(new Uint8Array([1, 2, 3]));

    const [, , first, second] = ws.sentJSON();
    assert.equal(first.type, "subscribe");
    assert.equal(first.payload.operationName, "Send");
    assert.deepEqual(first.payload.variables, {message: '{"action":"refresh"}'});
    assert.deepEqual(second.payload.variables, {b64: "AQID"});
    assert.notEqual(first.id, second.id);
});

test("ping is answered with pong", async () => {
    const {ws} = await connected();

    ws.serverSend(JSON.stringify({type: "ping"}));

    assert.deepEqual(ws.sentJSON().at(-1), {type: "pong"});
});

test("the server completing the events subscription ends the connection; a Send completing does not", async () => {
    const {c, ws, log} = await connected();
    const eventsID = ws.sentJSON()[1].id;

    c.send("{}");
    ws.serverSend(JSON.stringify({id: ws.sentJSON()[2].id, type: "complete"}));
    assert.equal(log.closes.length, 0);

    ws.serverSend(JSON.stringify({id: eventsID, type: "complete"}));
    assert.equal(log.closes.length, 1);
    assert.ok(ws.closed);
});

test("operation errors are reported through onError", async () => {
    const {ws, log} = await connected();

    ws.serverSend(JSON.stringify({id: "s1", type: "error", payload: [{message: "subscribe to IndriEvents before sending"}]}));

    assert.equal(log.errors.length, 1);
    assert.match(String(log.errors[0]), /IndriEvents/);
});

test("a close before connection_ack rejects connect", async () => {
    const c = new GraphQLWsTransportClient({url: "ws://server/graphql", WebSocket: FakeWebSocket});
    const opening = c.connect();
    const ws = FakeWebSocket.last();
    ws.serverOpen();
    ws.serverClose(4406, "Subprotocol not acceptable");

    await assert.rejects(opening, /Subprotocol/);
});
