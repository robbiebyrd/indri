import test from "node:test";
import assert from "node:assert/strict";

import {SseParser, type SseEvent} from "./sse-parser.ts";

function parse(...chunks: string[]): SseEvent[] {
    const events: SseEvent[] = [];
    const p = new SseParser((e) => events.push(e));
    for (const c of chunks) {
        p.push(c);
    }
    return events;
}

test("parses named and default events, skipping comments", () => {
    const events = parse(
        "event: connected\ndata: abc123\n\n",
        ": ping\n\n",
        'data: {"stage":{"currentScene":"login"}}\n\n',
    );

    assert.deepEqual(events, [
        {event: "connected", data: "abc123"},
        {event: "message", data: '{"stage":{"currentScene":"login"}}'},
    ]);
});

test("joins multiple data lines with newlines", () => {
    assert.deepEqual(parse("data: a\ndata: b\n\n"), [{event: "message", data: "a\nb"}]);
});

test("handles events split at arbitrary points and CRLF line endings", () => {
    const stream = "event: binary\r\ndata: AIGh/wrA\r\n\r\ndata: x\r\n\r\n";
    const whole = parse(stream);

    for (let cut = 0; cut <= stream.length; cut++) {
        assert.deepEqual(parse(stream.slice(0, cut), stream.slice(cut)), whole, `split at ${cut}`);
    }

    assert.deepEqual(whole, [
        {event: "binary", data: "AIGh/wrA"},
        {event: "message", data: "x"},
    ]);
});

test("a field with no space after the colon keeps its whole value", () => {
    assert.deepEqual(parse("data:tight\n\n"), [{event: "message", data: "tight"}]);
});

test("does not dispatch an event with no data", () => {
    assert.deepEqual(parse("event: lonely\n\n"), []);
});
