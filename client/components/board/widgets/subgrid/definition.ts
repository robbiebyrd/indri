/**
 * The `subgrid` widget's config contract, with no renderer attached.
 *
 * Unlike the other two widgets, the schema is NOT declared here: a sub-grid's
 * config is a nested layout, and `layout/schema/widget.ts` already owns that
 * shape because `parseLayout` has to walk it to apply the depth cap. This
 * module only pairs that schema with descriptors and defaults.
 */

import {MAX_SUBGRID_DEPTH, SUBGRID_TYPE, SubGridConfigSchema} from "../../../../layout/schema/widget.ts"

import type {FieldDescriptor} from "../../../../layout/registry/fields.ts"
import type {WidgetDefinition} from "../../../../layout/registry/registry.ts"
import type {SubGridConfig} from "../../../../layout/schema/widget.ts"

export {MAX_SUBGRID_DEPTH, SUBGRID_TYPE}

/**
 * `SubGridConfig` restated as a type ALIAS.
 *
 * `WidgetDefinition<C>` constrains `C extends Record<string, unknown>`, and
 * TypeScript grants implicit index signatures to aliases but not to
 * interfaces — `SubGridConfig` is an interface, so it cannot be used directly.
 * A homomorphic mapped type is used instead of retyping the members so the two
 * cannot drift: adding a key to `SubGridConfig` adds it here automatically.
 */
export type SubGridWidgetConfig = {[K in keyof SubGridConfig]: SubGridConfig[K]}

/**
 * Neither of a sub-grid's two keys fits the `FieldDescriptor` vocabulary,
 * which has no composite or child-collection kind (see `registry/fields.ts`:
 * every kind is one input control, and adding one is a control every panel
 * must then draw). They are described as text with an explicit note, and a
 * config panel is expected to special-case both rather than render a box.
 * Omitting them is not an option — `assertDescriptorsMatchSchema` requires a
 * descriptor for every key, precisely so nothing goes silently uneditable.
 */
const FIELDS: FieldDescriptor[] = [
    {
        key: "grid",
        label: "Grid size",
        kind: "text",
        required: true,
        description:
            "The nested coordinate space, as columns and rows (8-4096 each). A composite " +
            "value: a config panel should draw two number inputs, not a text box.",
    },
    {
        key: "widgets",
        label: "Child widgets",
        kind: "text",
        required: true,
        description:
            "The widgets placed inside this sub-grid, keyed by id. Edited on the canvas; " +
            "a config panel should skip this field rather than render it.",
    },
]

/**
 * The smallest legal sub-grid: `GridSizeSchema` floors both dimensions at 8,
 * and a new sub-grid starts empty because its children are placed on the
 * canvas rather than authored in a panel.
 */
export const SUBGRID_DEFAULTS: SubGridWidgetConfig = {
    grid: {cols: 8, rows: 8},
    widgets: {},
}

/** The renderer-free half of the definition. `index.tsx` adds `Component`. */
export const subGridDefinition: WidgetDefinition<SubGridWidgetConfig> = {
    type: SUBGRID_TYPE,
    schema: SubGridConfigSchema,
    fields: FIELDS,
    defaults: SUBGRID_DEFAULTS,
}
