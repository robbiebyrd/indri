import {z} from "zod"
import {registerWidget} from "./registry.ts"
import type {WidgetDefinition, WidgetContext} from "./registry.ts"
import {TextWidget} from "@/components/board/widgets/text/index.tsx"
import {ImageWidget} from "@/components/board/widgets/image/index.tsx"
import {SubgridWidget} from "@/components/board/widgets/subgrid/index.tsx"

// ---------------------------------------------------------------------------
// Text widget
// ---------------------------------------------------------------------------

export const TextConfig = z.object({
    text: z.string(),
    fontSize: z.number().positive().optional(),
    color: z.string().optional(),
    align: z.enum(["left", "center", "right"]).optional(),
}).strict()

export type TextConfig = z.infer<typeof TextConfig>

export const textWidgetDefinition: WidgetDefinition<TextConfig> = {
    type: "text",
    schema: TextConfig,
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
    Component: TextWidget,
    api: (ctx: WidgetContext) => ({
        setText: (t: unknown) => ctx.override({text: String(t)}),
    }),
}

// ---------------------------------------------------------------------------
// Image widget
// ---------------------------------------------------------------------------

export const ImageConfig = z.object({
    uri: z.string().url(),
    resizeMode: z.enum(["contain", "cover", "stretch", "repeat", "center"]).optional(),
}).strict()

export type ImageConfig = z.infer<typeof ImageConfig>

export const imageWidgetDefinition: WidgetDefinition<ImageConfig> = {
    type: "image",
    schema: ImageConfig,
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
    Component: ImageWidget,
    api: (ctx: WidgetContext) => ({
        setUri: (u: unknown) => ctx.override({uri: String(u)}),
    }),
}

// ---------------------------------------------------------------------------
// Sub-grid widget
// ---------------------------------------------------------------------------

export const SubgridConfig = z.object({
    grid: z.object({cols: z.number().int().positive(), rows: z.number().int().positive()}),
    widgets: z.record(z.string(), z.unknown()),
})

export type SubgridConfig = z.infer<typeof SubgridConfig>

export const subgridWidgetDefinition: WidgetDefinition<SubgridConfig> = {
    type: "subgrid",
    schema: SubgridConfig,
    fields: [],  // no config panel fields — config is structural, not user-editable
    defaults: {grid: {cols: 3, rows: 3}, widgets: {}},
    Component: SubgridWidget,
    api: (_ctx: WidgetContext) => ({}),
}

// ---------------------------------------------------------------------------
// Register all widgets — this side effect runs on import
// ---------------------------------------------------------------------------

registerWidget(textWidgetDefinition)
registerWidget(imageWidgetDefinition)
registerWidget(subgridWidgetDefinition)
