import {z} from "zod"

import {GridSizeSchema, Placement} from "./placement.ts"
import {StyleSchema} from "./style.ts"

/**
 * Maximum sub-grid nesting. A sub-grid renders a grid of widgets, any of which
 * may itself be a sub-grid, so an unbounded payload is a stack-overflow surface.
 * Depth is enforced by `parseLayout` rather than by the schema, because zod
 * cannot count recursion levels.
 */
export const MAX_SUBGRID_DEPTH = 4

/**
 * A widget as it appears on the wire.
 *
 * `config` is deliberately opaque here. Each widget type owns the shape of its
 * own config and validates it through the widget registry, so encoding those
 * shapes in this schema would duplicate the registry and go stale. This module
 * validates *structure* — placement, styling, identity — and nothing about what
 * a "text" or "image" widget means.
 */
export interface Widget {
    type: string
    placement: z.infer<typeof Placement>
    style?: z.infer<typeof StyleSchema>
    config?: Record<string, unknown>
    script?: string
}

export const WidgetSchema: z.ZodType<Widget> = z.object({
    type: z.string().min(1),
    placement: Placement,
    style: StyleSchema.optional(),
    config: z.record(z.string(), z.unknown()).optional(),
    script: z.string().optional(),
}).strict()

/**
 * The sub-grid widget's config: a nested coordinate space with its own widgets.
 *
 * Recursive via `z.lazy`, so a sub-grid can contain sub-grids. The depth cap is
 * applied by `parseLayout`; this schema only says the shape is well-formed.
 */
export interface SubGridConfig {
    grid: z.infer<typeof GridSizeSchema>
    widgets: Record<string, Widget>
}

export const SubGridConfigSchema: z.ZodType<SubGridConfig> = z.lazy(() =>
    z.object({
        grid: GridSizeSchema,
        // A MAP keyed by id, never an array: the server's events.Diff replaces
        // arrays whole, so an array here would re-send every child on any single
        // change and defeat the delta pipeline this engine is built on.
        widgets: z.record(z.string().min(1), WidgetSchema),
    }).strict(),
)

/** The widget `type` value that carries a nested grid. */
export const SUBGRID_TYPE = "subgrid"
