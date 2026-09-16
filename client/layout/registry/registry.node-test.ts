// Tests for the widget registry and field descriptors.
// IMPORTANT: No Zod internals (._def or ._zod.def) are used anywhere in this file.
// Bidirectional schema/field consistency is checked via `schema.shape` (the public
// ZodObject API) and the fields array.
import test from "node:test"
import assert from "node:assert/strict"

import {z} from "zod"
import type {WidgetDefinition} from "./registry.ts"
import {registerWidget, getWidget, getAllWidgets, resetRegistry} from "./registry.ts"
import type {FieldDescriptor} from "./fields.ts"

// ---------------------------------------------------------------------------
// Sample widget definitions (live here, not in registry.ts — real definitions
// travel with their components in client/components/board/widgets/*/)
// ---------------------------------------------------------------------------

const textSchema = z.object({
    text: z.string(),
    fontSize: z.number().positive().optional(),
    color: z.string().optional(),
    align: z.enum(["left", "center", "right"]).optional(),
}).strict()

const textDef: WidgetDefinition<z.infer<typeof textSchema>> = {
    type: "text",
    schema: textSchema,
    fields: [
        {key: "text",     label: "Text",   kind: "text", multiline: true},
        {key: "fontSize", label: "Size",   kind: "number", min: 8, max: 512},
        {key: "color",    label: "Colour", kind: "color"},
        {key: "align",    label: "Align",  kind: "select", options: [
            {value: "left",   label: "Left"},
            {value: "center", label: "Center"},
            {value: "right",  label: "Right"},
        ]},
    ],
    defaults: {text: ""},
    Component: null,
    api: () => ({}),
}

const imageSchema = z.object({
    uri: z.string().url(),
    resizeMode: z.enum(["contain", "cover", "stretch", "repeat", "center"]).optional(),
}).strict()

const imageDef: WidgetDefinition<z.infer<typeof imageSchema>> = {
    type: "image",
    schema: imageSchema,
    fields: [
        {key: "uri",        label: "Image URL", kind: "uri", accept: "image"},
        {key: "resizeMode", label: "Fit",        kind: "select", options: [
            {value: "contain", label: "Contain"},
            {value: "cover",   label: "Cover"},
        ]},
    ],
    defaults: {uri: "https://example.com/placeholder.png"},
    Component: null,
    api: () => ({}),
}

// ---------------------------------------------------------------------------
// Helper: check bidirectional schema ↔ fields consistency for a ZodObject def
// ---------------------------------------------------------------------------
function assertBidirectional<C>(def: WidgetDefinition<C>): void {
    const shape = (def.schema as z.ZodObject<z.ZodRawShape>).shape
    const schemaKeys = Object.keys(shape)
    const fieldKeys = def.fields.map((f: FieldDescriptor) => f.key)

    // Every schema key has a matching field descriptor
    for (const key of schemaKeys) {
        assert.ok(
            fieldKeys.includes(key),
            `schema key "${key}" has no matching field descriptor in widget "${def.type}"`,
        )
    }

    // Every field descriptor key exists in the schema shape
    for (const key of fieldKeys) {
        assert.ok(
            key in shape,
            `field key "${key}" has no matching key in schema.shape for widget "${def.type}"`,
        )
    }
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

test("bidirectional check: text widget schema keys match field descriptors", () => {
    assertBidirectional(textDef)
})

test("bidirectional check: image widget schema keys match field descriptors", () => {
    assertBidirectional(imageDef)
})

test("registering a duplicate widget type throws", () => {
    resetRegistry()
    registerWidget(textDef)
    assert.throws(
        () => registerWidget(textDef),
        /already registered/,
    )
})

test("text widget defaults validate against its schema", () => {
    const result = textSchema.safeParse(textDef.defaults)
    assert.equal(result.success, true, `text defaults failed validation: ${JSON.stringify(!result.success && result.error?.issues)}`)
})

test("image widget defaults validate against its schema", () => {
    const result = imageSchema.safeParse(imageDef.defaults)
    assert.equal(result.success, true, `image defaults failed validation: ${JSON.stringify(!result.success && result.error?.issues)}`)
})

test("uri field descriptor has accept discriminator set to 'image'", () => {
    const uriField = imageDef.fields.find(f => f.key === "uri")
    assert.ok(uriField, "uri field should exist on image widget")
    assert.equal(uriField.kind, "uri")
    // Narrow to uri kind — accept discriminator is present without schema changes
    assert.equal((uriField as Extract<FieldDescriptor, {kind: "uri"}>).accept, "image")
})

test("resetRegistry clears state so re-registration does not throw", () => {
    resetRegistry()
    // Should not throw — registry is empty after reset
    assert.doesNotThrow(() => registerWidget(textDef))
})

test("getWidget returns the registered definition", () => {
    resetRegistry()
    registerWidget(textDef)
    const retrieved = getWidget("text")
    assert.ok(retrieved, "getWidget should return the registered definition")
    assert.equal(retrieved.type, "text")
    assert.equal(retrieved.fields.length, textDef.fields.length)
})

test("getAllWidgets returns all registered definitions", () => {
    resetRegistry()
    registerWidget(textDef)
    registerWidget(imageDef)
    const all = getAllWidgets()
    assert.equal(all.length, 2)
    const types = all.map(d => d.type)
    assert.ok(types.includes("text"))
    assert.ok(types.includes("image"))
})

test("getWidget returns undefined for an unregistered type", () => {
    resetRegistry()
    assert.equal(getWidget("nonexistent"), undefined)
})
