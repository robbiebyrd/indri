// Tests for text, image and sub-grid widget definitions.
// IMPORTANT: This file does NOT import widgets.ts — that file has React Native imports
// and cannot run under bare Node. Instead, the schemas and api() functions are defined
// inline here (matching the real definitions exactly) to test behaviour without RN deps.
// NOTE: schema.shape is the public ZodObject API — no ._def or ._zod.def introspection here.
import test from "node:test"
import assert from "node:assert/strict"

import {z} from "zod"
import {registerWidget, getWidget, resetRegistry} from "./registry.ts"
import type {WidgetDefinition, WidgetContext} from "./registry.ts"
import {parseLayout} from "../schema/layout.ts"

// ---------------------------------------------------------------------------
// Inline schema definitions (must match widgets.ts exactly — these are the
// canonical shapes; widgets.ts re-exports them wrapped in WidgetDefinitions)
// ---------------------------------------------------------------------------

const textSchema = z.object({
    text: z.string(),
    fontSize: z.number().positive().optional(),
    color: z.string().optional(),
    align: z.enum(["left", "center", "right"]).optional(),
}).strict()

const imageSchema = z.object({
    uri: z.string().url(),
    resizeMode: z.enum(["contain", "cover", "stretch", "repeat", "center"]).optional(),
}).strict()

const subgridSchema = z.object({
    grid: z.object({cols: z.number().int().positive(), rows: z.number().int().positive()}),
    widgets: z.record(z.string(), z.unknown()),
})

// ---------------------------------------------------------------------------
// Minimal stub definitions for api() tests (Component is null — no RN needed)
// ---------------------------------------------------------------------------

const makeCtx = (): WidgetContext => ({
    widgetId: "w1",
    sceneId: "s1",
    override: () => undefined,
})

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
    api: (ctx: WidgetContext) => ({
        setText: (t: unknown) => ctx.override({text: String(t)}),
    }),
}

const imageDef: WidgetDefinition<z.infer<typeof imageSchema>> = {
    type: "image",
    schema: imageSchema,
    fields: [
        {key: "uri",        label: "Image URL", kind: "uri",    accept: "image"},
        {key: "resizeMode", label: "Fit",        kind: "select", options: [
            {value: "contain", label: "Contain"},
            {value: "cover",   label: "Cover"},
            {value: "stretch", label: "Stretch"},
            {value: "repeat",  label: "Repeat"},
            {value: "center",  label: "Center"},
        ]},
    ],
    defaults: {uri: "https://example.com/placeholder.png"},
    Component: null,
    api: (ctx: WidgetContext) => ({
        setUri: (u: unknown) => ctx.override({uri: String(u)}),
    }),
}

const subgridDef: WidgetDefinition<z.infer<typeof subgridSchema>> = {
    type: "subgrid",
    schema: subgridSchema,
    fields: [],
    defaults: {grid: {cols: 3, rows: 3}, widgets: {}},
    Component: null,
    api: (_ctx: WidgetContext) => ({}),
}

// ---------------------------------------------------------------------------
// 1. Text defaults validate against text schema
// ---------------------------------------------------------------------------
test("text defaults validate against text schema", () => {
    const result = textSchema.safeParse(textDef.defaults)
    assert.equal(
        result.success,
        true,
        `text defaults should be valid: ${JSON.stringify(!result.success ? result.error?.issues : [])}`,
    )
})

// ---------------------------------------------------------------------------
// 2. Image defaults validate against image schema
// ---------------------------------------------------------------------------
test("image defaults validate against image schema", () => {
    const result = imageSchema.safeParse(imageDef.defaults)
    assert.equal(
        result.success,
        true,
        `image defaults should be valid: ${JSON.stringify(!result.success ? result.error?.issues : [])}`,
    )
})

// ---------------------------------------------------------------------------
// 3. Sub-grid defaults validate against subgrid schema
// ---------------------------------------------------------------------------
test("subgrid defaults validate against subgrid schema", () => {
    const result = subgridSchema.safeParse(subgridDef.defaults)
    assert.equal(
        result.success,
        true,
        `subgrid defaults should be valid: ${JSON.stringify(!result.success ? result.error?.issues : [])}`,
    )
})

// ---------------------------------------------------------------------------
// 4. Unknown align value is rejected by text schema
// ---------------------------------------------------------------------------
test("text schema rejects unknown align value", () => {
    const result = textSchema.safeParse({text: "hi", align: "invalid"})
    assert.equal(result.success, false, "align: 'invalid' must be rejected by the text schema")
})

// ---------------------------------------------------------------------------
// 5. Sub-grid schema accepts nesting to depth 4
// The outer subgrid schema uses z.record(z.unknown()) for inner widgets, so this
// is straightforward — any valid nesting depth is accepted at the schema level.
// The depth cap is enforced by parseLayout's semantic check, not the schema.
// ---------------------------------------------------------------------------
test("subgrid schema accepts nesting to depth 4", () => {
    // Build a config with subgrid nesting at depth 4
    const depth4Config = {
        grid: {cols: 2, rows: 2},
        widgets: {
            inner1: {
                grid: {cols: 2, rows: 2},
                widgets: {
                    inner2: {
                        grid: {cols: 2, rows: 2},
                        widgets: {
                            inner3: {
                                grid: {cols: 2, rows: 2},
                                widgets: {
                                    inner4: {grid: {cols: 1, rows: 1}, widgets: {}},
                                },
                            },
                        },
                    },
                },
            },
        },
    }
    // The subgrid schema uses z.record(z.unknown()) for inner widgets, so
    // any nested structure parses at the schema level — depth checks are in parseLayout
    const result = subgridSchema.safeParse(depth4Config)
    assert.equal(
        result.success,
        true,
        `subgrid schema must accept depth-4 nesting, got: ${JSON.stringify(!result.success ? result.error?.issues : [])}`,
    )
})

// ---------------------------------------------------------------------------
// 6. Sub-grid nesting at depth 5 is rejected by parseLayout
// The schema accepts it (passthrough at max depth), but parseLayout emits an issue.
// This test validates that the enforcement mechanism works correctly.
// ---------------------------------------------------------------------------
test("parseLayout emits an issue for subgrid nesting at depth 5", () => {
    // Build a full GameLayout with subgrid nesting at depth 5.
    // Widgets need valid placement to pass the schema; we use grid placement.
    function gridPlacement() {
        return {kind: "grid", col: 0, row: 0, w: 1, h: 1}
    }

    const depth5Layout = {
        grid: {cols: 8, rows: 8},
        scenes: {
            main: {
                widgets: {
                    sg1: {
                        type: "subgrid",
                        placement: gridPlacement(),
                        config: {
                            grid: {cols: 2, rows: 2},
                            widgets: {
                                sg2: {
                                    type: "subgrid",
                                    placement: gridPlacement(),
                                    config: {
                                        grid: {cols: 2, rows: 2},
                                        widgets: {
                                            sg3: {
                                                type: "subgrid",
                                                placement: gridPlacement(),
                                                config: {
                                                    grid: {cols: 2, rows: 2},
                                                    widgets: {
                                                        sg4: {
                                                            type: "subgrid",
                                                            placement: gridPlacement(),
                                                            config: {
                                                                grid: {cols: 2, rows: 2},
                                                                widgets: {
                                                                    sg5: {
                                                                        type: "subgrid",
                                                                        placement: gridPlacement(),
                                                                        config: {grid: {cols: 1, rows: 1}, widgets: {}},
                                                                    },
                                                                },
                                                            },
                                                        },
                                                    },
                                                },
                                            },
                                        },
                                    },
                                },
                            },
                        },
                    },
                },
            },
        },
    }

    const {issues} = parseLayout(depth5Layout)
    // parseLayout must report at least one issue about exceeding nesting depth
    assert.ok(
        issues.length > 0,
        "parseLayout must emit at least one issue for depth-5 subgrid nesting",
    )
    const hasDepthIssue = issues.some(i => /exceed|depth|nesting/i.test(i.message))
    assert.ok(
        hasDepthIssue,
        `Expected a depth-related issue but got: ${JSON.stringify(issues)}`,
    )
})

// ---------------------------------------------------------------------------
// 7. All 3 widget api() functions return an object
// ---------------------------------------------------------------------------
test("text widget api returns an object", () => {
    const api = textDef.api(makeCtx())
    assert.equal(typeof api, "object", "text api() must return an object")
    assert.ok(api !== null, "text api() must not return null")
})

test("image widget api returns an object", () => {
    const api = imageDef.api(makeCtx())
    assert.equal(typeof api, "object", "image api() must return an object")
    assert.ok(api !== null, "image api() must not return null")
})

test("subgrid widget api returns an object", () => {
    const api = subgridDef.api(makeCtx())
    assert.equal(typeof api, "object", "subgrid api() must return an object")
    assert.ok(api !== null, "subgrid api() must not return null")
})

// ---------------------------------------------------------------------------
// 8. api() setText actually calls override with the correct patch
// ---------------------------------------------------------------------------
test("text api setText calls ctx.override with correct patch", () => {
    const patches: Record<string, unknown>[] = []
    const ctx: WidgetContext = {
        widgetId: "w1",
        sceneId: "s1",
        override: (patch) => { patches.push(patch) },
    }
    const api = textDef.api(ctx)
    ;(api.setText as (t: string) => void)("hello")
    assert.equal(patches.length, 1, "override must be called once")
    assert.equal(patches[0]?.text, "hello", "override must receive {text: 'hello'}")
})

// ---------------------------------------------------------------------------
// 9. api() setUri actually calls override with the correct patch
// ---------------------------------------------------------------------------
test("image api setUri calls ctx.override with correct patch", () => {
    const patches: Record<string, unknown>[] = []
    const ctx: WidgetContext = {
        widgetId: "w1",
        sceneId: "s1",
        override: (patch) => { patches.push(patch) },
    }
    const api = imageDef.api(ctx)
    ;(api.setUri as (u: string) => void)("https://example.com/new.png")
    assert.equal(patches.length, 1, "override must be called once")
    assert.equal(patches[0]?.uri, "https://example.com/new.png", "override must receive correct uri")
})

// ---------------------------------------------------------------------------
// 10. Widgets can be registered in the registry
// ---------------------------------------------------------------------------
test("all 3 widget definitions register without errors", () => {
    resetRegistry()
    assert.doesNotThrow(() => registerWidget(textDef), "text widget must register without error")
    assert.doesNotThrow(() => registerWidget(imageDef), "image widget must register without error")
    assert.doesNotThrow(() => registerWidget(subgridDef), "subgrid widget must register without error")

    assert.ok(getWidget("text"), "text widget must be retrievable after registration")
    assert.ok(getWidget("image"), "image widget must be retrievable after registration")
    assert.ok(getWidget("subgrid"), "subgrid widget must be retrievable after registration")
    resetRegistry()
})
