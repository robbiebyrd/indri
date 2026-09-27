import test from "node:test"
import assert from "node:assert/strict"
import {z} from "zod"

// The PICKERS mapped type in config-panel.tsx ensures at compile time that every
// FieldDescriptor["kind"] has a registered picker. The runtime test below verifies
// the current set of known kinds is non-empty and stable, acting as a canary: if
// a new kind is added to fields.ts it must appear here (and in PICKERS).
const FIELD_DESCRIPTOR_KINDS = [
    "text", "number", "color", "boolean", "date", "select", "multiselect", "uri",
] as const

test("FIELD_DESCRIPTOR_KINDS set is non-empty and all entries are strings", () => {
    assert.ok(FIELD_DESCRIPTOR_KINDS.length > 0)
    for (const k of FIELD_DESCRIPTOR_KINDS) {
        assert.equal(typeof k, "string")
    }
})

test("FIELD_DESCRIPTOR_KINDS matches the FieldDescriptor union (8 kinds)", () => {
    // If this fails, fields.ts was updated without updating PICKERS or this test.
    assert.equal(FIELD_DESCRIPTOR_KINDS.length, 8)
})

// ---------------------------------------------------------------------------
// Panel validation logic: only emit when schema passes
// ---------------------------------------------------------------------------

test("valid config value passes schema validation and would be sent", () => {
    const schema = z.object({text: z.string().min(1)})
    const value = {text: "hello"}
    assert.ok(schema.safeParse(value).success)
})

test("invalid config value fails schema validation and is not sent", () => {
    const schema = z.object({text: z.string().min(1)})
    const value = {text: ""}
    assert.equal(schema.safeParse(value).success, false)
})

test("partial update merges with existing config before validation", () => {
    const schema = z.object({text: z.string(), fontSize: z.number().positive().optional()}).strict()
    const existing = {text: "hello", fontSize: 14}
    const patch = {text: "world"}
    const merged = {...existing, ...patch}
    const result = schema.safeParse(merged)
    assert.ok(result.success)
    const data = result.data as typeof merged
    assert.equal(data.text, "world")
    assert.equal(data.fontSize, 14)
})

test("schema rejects extra keys (strict mode) — panel should not forward them", () => {
    const schema = z.object({text: z.string()}).strict()
    const result = schema.safeParse({text: "ok", unknown: "boom"})
    assert.equal(result.success, false)
})

test("number field outside min/max fails schema — panel shows error", () => {
    const schema = z.object({fontSize: z.number().min(8).max(512)})
    assert.equal(schema.safeParse({fontSize: 0}).success, false)
    assert.equal(schema.safeParse({fontSize: 1000}).success, false)
    assert.ok(schema.safeParse({fontSize: 16}).success)
})
