import {z} from "zod"
import {StyleSchema} from "./style.ts"
import {Placement} from "./placement.ts"

// Base fields shared by all widgets
const WidgetBase = z.object({
    placement: Placement,
    style: StyleSchema.optional(),
})

// Text widget
const TextConfig = z.object({
    text: z.string(),
    fontSize: z.number().positive().optional(),
    color: z.string().optional(),
    align: z.enum(["left", "center", "right"]).optional(),
}).strict()

const TextWidget = WidgetBase.extend({type: z.literal("text"), config: TextConfig})

// Image widget
const ImageConfig = z.object({
    uri: z.string().url(),
    resizeMode: z.enum(["contain", "cover", "stretch", "repeat", "center"]).optional(),
}).strict()

const ImageWidget = WidgetBase.extend({type: z.literal("image"), config: ImageConfig})

// Subgrid widget — recursive, depth-capped to MAX_SUBGRID_DEPTH.
// At depth 0 the passthrough variant is used: placement is still validated but config
// is accepted as-is. parseLayout detects this case and emits an issue.
function makeSubgridWidget(depth: number): z.ZodTypeAny {
    if (depth <= 0) {
        // At max depth: accept anything — further nesting is caught by the semantic check
        return WidgetBase.extend({
            type: z.literal("subgrid"),
        }).passthrough()
    }
    const SubgridConfig = z.object({
        grid: z.object({
            cols: z.number().int().positive(),
            rows: z.number().int().positive(),
        }),
        widgets: z.record(z.string(), z.lazy(() => makeWidgetSchema(depth - 1))),
    })
    return WidgetBase.extend({type: z.literal("subgrid"), config: SubgridConfig})
}

function makeWidgetSchema(depth: number): z.ZodTypeAny {
    if (depth <= 0) {
        // Beyond max depth, only placement is validated; type must still be present
        return z.object({type: z.string(), placement: Placement}).passthrough()
    }
    return z.discriminatedUnion("type", [
        TextWidget,
        ImageWidget,
        makeSubgridWidget(depth) as z.ZodObject<z.ZodRawShape>,
    ])
}

export const MAX_SUBGRID_DEPTH = 4
export const WidgetSchema = makeWidgetSchema(MAX_SUBGRID_DEPTH)
export type Widget = z.infer<typeof WidgetSchema>
