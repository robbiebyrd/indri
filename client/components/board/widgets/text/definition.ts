/**
 * The `text` widget's config contract, with no renderer attached.
 *
 * Split from `index.tsx` on purpose. The bare-Node test runner
 * (`node --experimental-strip-types`) cannot load JSX, and `react-native` is
 * not loadable outside a bundler at all, so a definition that imported its own
 * component would be untestable. Everything here is zod and plain data, which
 * is exactly the part the tests need.
 */

import {z} from "zod"

import type {FieldDescriptor} from "../../../../layout/registry/fields.ts"
import type {WidgetDefinition} from "../../../../layout/registry/registry.ts"

/** Matches `Widget.type` on the wire. */
export const TEXT_TYPE = "text"

/**
 * `.strict()` because the registry demands it: a loose schema would silently
 * drop a misspelled config key instead of reporting it, and would also make
 * `assertDescriptorsMatchSchema` pass vacuously.
 */
export const TextConfigSchema = z.object({
    text: z.string(),
    // Positive rather than merely non-negative: a zero font size renders
    // nothing at all, which is indistinguishable from a missing widget.
    fontSize: z.number().positive().optional(),
    // Plain string, not a colour format check. RN accepts named colours,
    // `#rgb`, `#rrggbb`, `rgba(...)` and `hsl(...)`, and re-deriving that
    // grammar here would reject values the platform handles perfectly well.
    color: z.string().optional(),
    align: z.enum(["left", "center", "right"]).optional(),
    // Only the two weights every platform has a real font file for. RN's
    // numeric weights fall back unpredictably when the family lacks the face.
    weight: z.enum(["normal", "bold"]).optional(),
}).strict()

/**
 * Derived with `z.infer`, never declared as an `interface`.
 *
 * `WidgetDefinition<C>` constrains `C extends Record<string, unknown>` so the
 * concrete definition can live in the registry's erased map, and TypeScript
 * gives implicit index signatures to type ALIASES only. An interface here
 * would not compile.
 */
export type TextConfig = z.infer<typeof TextConfigSchema>

const ALIGN_OPTIONS = [
    {label: "Left", value: "left"},
    {label: "Centre", value: "center"},
    {label: "Right", value: "right"},
]

const WEIGHT_OPTIONS = [
    {label: "Normal", value: "normal"},
    {label: "Bold", value: "bold"},
]

const FIELDS: FieldDescriptor[] = [
    {key: "text", label: "Text", kind: "text", multiline: true, required: true},
    {key: "fontSize", label: "Size", kind: "number", min: 1, max: 512, step: 1},
    {key: "color", label: "Colour", kind: "color"},
    {key: "align", label: "Align", kind: "select", options: ALIGN_OPTIONS},
    {key: "weight", label: "Weight", kind: "select", options: WEIGHT_OPTIONS},
]

/**
 * Every optional key is present in `defaults`, not just the required one.
 *
 * `assertDescriptorsMatchSchema` enumerates the schema's key set as
 * `Object.keys(defaults)`, so an optional key omitted here is invisible to the
 * descriptor check. Listing them all is what keeps that check exhaustive.
 */
export const TEXT_DEFAULTS: TextConfig = {
    text: "",
    fontSize: 16,
    color: "#111111",
    align: "left",
    weight: "normal",
}

/** The renderer-free half of the definition. `index.tsx` adds `Component`. */
export const textDefinition: WidgetDefinition<TextConfig> = {
    type: TEXT_TYPE,
    schema: TextConfigSchema,
    fields: FIELDS,
    defaults: TEXT_DEFAULTS,
}
