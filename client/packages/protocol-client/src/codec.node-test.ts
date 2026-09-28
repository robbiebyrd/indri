import test from "node:test";
import assert from "node:assert/strict";

import {base64Decode, base64Encode, Utf8StreamDecoder} from "./codec.ts";

test("base64 round-trips arbitrary bytes, including padding cases", () => {
    for (const len of [0, 1, 2, 3, 4, 5, 255]) {
        const bytes = new Uint8Array(len).map((_, i) => (i * 37 + 11) & 0xff);
        const encoded = base64Encode(bytes);
        assert.equal(encoded, Buffer.from(bytes).toString("base64"), `encode len ${len}`);
        assert.deepEqual(base64Decode(encoded), bytes, `decode len ${len}`);
    }
});

test("base64Decode matches the server's standard encoding", () => {
    // Go: base64.StdEncoding.EncodeToString([]byte{0x00, 0x81, 0xa1, 0xff, '\n', 0xc0})
    assert.deepEqual(base64Decode("AIGh/wrA"), new Uint8Array([0x00, 0x81, 0xa1, 0xff, 0x0a, 0xc0]));
});

test("base64Decode rejects invalid input instead of returning garbage", () => {
    assert.throws(() => base64Decode("a$b="));
    assert.throws(() => base64Decode("abc"));
});

test("Utf8StreamDecoder decodes multibyte characters split across chunks", () => {
    const text = "héllo → 世界 🎲";
    const bytes = new TextEncoder().encode(text);

    // Every possible single split point, so each multibyte sequence is cut
    // at every offset at least once.
    for (let cut = 0; cut <= bytes.length; cut++) {
        const d = new Utf8StreamDecoder();
        const out = d.decode(bytes.subarray(0, cut)) + d.decode(bytes.subarray(cut));
        assert.equal(out, text, `split at ${cut}`);
    }

    const d = new Utf8StreamDecoder();
    let out = "";
    for (const b of bytes) {
        out += d.decode(new Uint8Array([b]));
    }
    assert.equal(out, text, "byte at a time");
});

test("Utf8StreamDecoder replaces invalid bytes rather than throwing", () => {
    const d = new Utf8StreamDecoder();
    assert.equal(d.decode(new Uint8Array([0x61, 0xff, 0x62])), "a\uFFFDb");
});
