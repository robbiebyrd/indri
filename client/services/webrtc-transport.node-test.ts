// Pure-logic regression tests backing webrtc-transport.ts.
//
// There is no WebRTC implementation available to `node --experimental-strip-types`
// (see pnpm test in package.json), so a real RTCPeerConnection/RTCDataChannel
// cannot be exercised here. This file covers exactly the parts of Step 11
// (plans/webrtc-transport.md) that do not require one: the signal envelope
// the client builds and parses, the /rtc/offer URL derivation, the binary/text
// payload normalisation (criterion 5), and the gather-complete wait (criterion
// 2), driven with a fake object exposing iceGatheringState and an event hook
// rather than a mocked RTCPeerConnection. The PeerConnection itself — does
// gathering actually happen, does the DataChannel actually reach "open" on
// web/iOS/Android — is only verifiable on hardware and is covered by the
// manual spike, story 042-38ee.
//
// These helpers are imported from webrtc-signal.ts, NOT webrtc-transport.ts:
// the latter has a top-level value import of react-native-webrtc-web-shim,
// whose own internals Node's ESM loader cannot resolve at all (extensionless
// relative imports only Metro follows) — importing anything from that file,
// even a named export that never touches the shim, crashes this test runner.
// webrtc-transport.ts re-exports nothing from webrtc-signal.ts for that same
// reason; it only consumes these functions internally.
import test from "node:test";
import assert from "node:assert/strict";

import {
    ChunkReassembler,
    buildOfferSignal,
    normaliseMessage,
    parseSignal,
    parseSignalFromJson,
    signalUrl,
    waitForIceGatheringComplete,
} from "./webrtc-signal.ts";

import type {GatherableConnection} from "./webrtc-signal.ts";

test("buildOfferSignal keeps only type and sdp from a local description", () => {
    const signal = buildOfferSignal({type: "offer", sdp: "v=0\r\n"});
    assert.deepStrictEqual(signal, {type: "offer", sdp: "v=0\r\n"});
});

test("parseSignal accepts a valid answer envelope", () => {
    const signal = parseSignal({type: "answer", sdp: "v=0\r\n"});
    assert.deepStrictEqual(signal, {type: "answer", sdp: "v=0\r\n"});
});

test("parseSignal carries an optional peerId through when present", () => {
    const signal = parseSignal({type: "offer", sdp: "v=0\r\n", peerId: "42"});
    assert.deepStrictEqual(signal, {type: "offer", sdp: "v=0\r\n", peerId: "42"});
});

test("parseSignal returns undefined rather than throwing on non-object input", () => {
    assert.equal(parseSignal(null), undefined);
    assert.equal(parseSignal(undefined), undefined);
    assert.equal(parseSignal("not an object"), undefined);
    assert.equal(parseSignal(42), undefined);
});

test("parseSignal returns undefined when type or sdp is missing or the wrong type", () => {
    assert.equal(parseSignal({sdp: "v=0\r\n"}), undefined, "missing type");
    assert.equal(parseSignal({type: "answer"}), undefined, "missing sdp");
    assert.equal(parseSignal({type: 1, sdp: "v=0\r\n"}), undefined, "non-string type");
    assert.equal(parseSignal({type: "answer", sdp: null}), undefined, "non-string sdp");
});

test("parseSignalFromJson parses a well-formed /rtc/offer response body", () => {
    const signal = parseSignalFromJson(JSON.stringify({type: "answer", sdp: "v=0\r\n"}));
    assert.deepStrictEqual(signal, {type: "answer", sdp: "v=0\r\n"});
});

test("parseSignalFromJson returns undefined rather than throwing on malformed JSON", () => {
    // parseJsonSafely already logs and swallows the SyntaxError; assert it
    // doesn't reach here as a thrown exception.
    const error = console.error;
    let logged = 0;
    console.error = () => {logged++};
    try {
        assert.equal(parseSignalFromJson("{not json"), undefined);
    } finally {
        console.error = error;
    }
    assert.equal(logged, 1, "the parse failure is logged, not silent");
});

test("signalUrl derives the fixed signalling route from a ws:// url, ignoring its path", () => {
    assert.equal(signalUrl("ws://localhost:5004/ws"), "http://localhost:5004/rtc/offer");
});

test("signalUrl maps wss:// to https://", () => {
    assert.equal(signalUrl("wss://example.com/ws?x=1"), "https://example.com/rtc/offer");
});

/**
 * `Uint8Array.prototype.buffer` is typed `ArrayBufferLike` (it can in theory
 * be backed by a SharedArrayBuffer), but normaliseMessage only accepts a
 * concrete `ArrayBuffer` — same as react-native-webrtc and the browser both
 * hand it. This builds a real one, matching what either platform delivers.
 */
function toArrayBuffer(text: string): ArrayBuffer {
    const bytes = new TextEncoder().encode(text);
    const buf = new ArrayBuffer(bytes.byteLength);
    new Uint8Array(buf).set(bytes);
    return buf;
}

test("normaliseMessage passes a string straight through", () => {
    assert.equal(normaliseMessage("hello", new ChunkReassembler()), "hello");
});

test("normaliseMessage decodes an unchunked ArrayBuffer to the string it encodes (criterion 4)", () => {
    assert.equal(normaliseMessage(toArrayBuffer("hello"), new ChunkReassembler()), "hello");
});

test("normaliseMessage produces the SAME string for equivalent string and ArrayBuffer payloads", () => {
    // This is the point of criterion 5: web and native must not diverge on
    // the JS type a consumer sees for the same logical message.
    const text = '{"op":"update","ts":"2020-01-01T00:00:00.000Z"}';
    assert.equal(
        normaliseMessage(text, new ChunkReassembler()),
        normaliseMessage(toArrayBuffer(text), new ChunkReassembler()),
    );
});

/**
 * Builds one chunk frame exactly as internal/transport/webrtc/conn.go's
 * sendChunked does: a 2-byte header (marker byte 0, then 0=continue/1=final)
 * followed by the raw payload bytes.
 */
function chunkFrame(final: boolean, payload: Uint8Array): ArrayBuffer {
    const frame = new Uint8Array(2 + payload.length);
    frame[0] = 0;
    frame[1] = final ? 1 : 0;
    frame.set(payload, 2);
    return frame.buffer;
}

test("ChunkReassembler.accept passes an unchunked payload through unchanged (criterion 4)", () => {
    const bytes = new TextEncoder().encode('{"a":1}');
    assert.deepStrictEqual(new ChunkReassembler().accept(bytes), bytes);
});

// Criterion 1, the load-bearing test: a payload comfortably over any real
// ceiling, containing a multi-byte UTF-8 character ('é', the 2-byte sequence
// C3 A9) repeated so it inevitably straddles some chunk boundary given a
// chunk payload size (37) that shares no common factor with the character's
// byte width. A reassembler that decoded each chunk to a string before
// concatenating -- instead of joining raw bytes first -- would corrupt the
// rune at whatever boundary it landed on and this would fail.
test("normaliseMessage reassembles a chunked payload into IDENTICAL bytes, including multi-byte UTF-8 split across a chunk boundary", () => {
    const original = JSON.stringify({greeting: "café ".repeat(2000)});
    const bytes = new TextEncoder().encode(original);
    assert.ok(bytes.length > 10000, "test payload should be comfortably large");

    const chunkPayloadSize = 37;
    const reassembler = new ChunkReassembler();

    let result: string | undefined;
    for (let offset = 0; offset < bytes.length; offset += chunkPayloadSize) {
        const end = Math.min(offset + chunkPayloadSize, bytes.length);
        const isFinal = end >= bytes.length;
        const message = normaliseMessage(chunkFrame(isFinal, bytes.subarray(offset, end)), reassembler);

        if (isFinal) {
            result = message;
        } else {
            assert.equal(message, undefined, "a non-final chunk must not yet produce a message");
        }
    }

    assert.equal(result, original);
});

test("normaliseMessage reassembles a two-chunk sequence exactly", () => {
    const original = "0123456789";
    const bytes = new TextEncoder().encode(original);
    const reassembler = new ChunkReassembler();

    assert.equal(normaliseMessage(chunkFrame(false, bytes.subarray(0, 6)), reassembler), undefined);
    assert.equal(normaliseMessage(chunkFrame(true, bytes.subarray(6)), reassembler), original);
});

test("ChunkReassembler bounds a sequence that never sends a final chunk, instead of growing forever", () => {
    const reassembler = new ChunkReassembler(10);

    reassembler.accept(new Uint8Array(chunkFrame(false, new Uint8Array(6)))); // 6 <= 10, buffered
    assert.throws(
        () => reassembler.accept(new Uint8Array(chunkFrame(false, new Uint8Array(6)))), // 12 > 10
        /exceeded 10 bytes/,
    );
});

test("ChunkReassembler recovers after exceeding its bound: the next message still passes through", () => {
    const reassembler = new ChunkReassembler(10);

    assert.throws(() => {
        reassembler.accept(new Uint8Array(chunkFrame(false, new Uint8Array(6))));
        reassembler.accept(new Uint8Array(chunkFrame(false, new Uint8Array(6))));
    });

    const bytes = new TextEncoder().encode("ok");
    assert.deepStrictEqual(reassembler.accept(bytes), bytes);
});

test("two ChunkReassembler instances never share state (a connection's partial sequence cannot leak into another)", () => {
    const a = new ChunkReassembler();
    const b = new ChunkReassembler();

    const bytes = new TextEncoder().encode("hello");
    assert.equal(a.accept(new Uint8Array(chunkFrame(false, bytes))), undefined);

    // b has never seen a's in-progress sequence, so an unrelated, unchunked
    // message on b must pass straight through rather than being treated as
    // a continuation.
    const unrelated = new TextEncoder().encode("unrelated");
    assert.deepStrictEqual(b.accept(unrelated), unrelated);
});

/** Drives waitForIceGatheringComplete without a real RTCPeerConnection. */
class FakeConnection implements GatherableConnection {
    iceGatheringState = "new";
    private listeners: (() => void)[] = [];

    addEventListener(_type: "icegatheringstatechange", listener: () => void): void {
        this.listeners.push(listener);
    }

    removeEventListener(_type: "icegatheringstatechange", listener: () => void): void {
        this.listeners = this.listeners.filter((l) => l !== listener);
    }

    setState(state: string): void {
        this.iceGatheringState = state;
        for (const listener of [...this.listeners]) {
            listener();
        }
    }
}

test("waitForIceGatheringComplete resolves immediately if gathering is already complete", async () => {
    const conn = new FakeConnection();
    conn.iceGatheringState = "complete";
    await waitForIceGatheringComplete(conn, 50);
});

test("waitForIceGatheringComplete resolves once the connection reports complete", async () => {
    const conn = new FakeConnection();
    const p = waitForIceGatheringComplete(conn, 1000);
    conn.setState("gathering");
    conn.setState("complete");
    await p;
});

test("waitForIceGatheringComplete ignores a state change that isn't complete yet", async () => {
    const conn = new FakeConnection();
    const p = waitForIceGatheringComplete(conn, 1000);
    conn.setState("gathering");
    // Give the event loop a tick; the promise must still be pending.
    let settled = false;
    p.then(() => {settled = true});
    await new Promise((r) => setTimeout(r, 10));
    assert.equal(settled, false, "gathering (not complete) must not resolve the wait");
    conn.setState("complete");
    await p;
});

test("waitForIceGatheringComplete rejects rather than hanging forever if gathering never completes", async () => {
    const conn = new FakeConnection();
    await assert.rejects(
        () => waitForIceGatheringComplete(conn, 20),
        /ice gathering did not complete/,
    );
});

test("waitForIceGatheringComplete detaches its listener once settled (no leak on repeated state changes)", async () => {
    const conn = new FakeConnection();
    const p = waitForIceGatheringComplete(conn, 1000);
    conn.setState("complete");
    await p;

    // A further, unrelated state change after settling must not throw or
    // re-invoke anything: the listener was removed on cleanup.
    assert.doesNotThrow(() => conn.setState("new"));
});
