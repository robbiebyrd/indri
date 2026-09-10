/**
 * The `image` widget's config contract, with no renderer attached.
 *
 * Split from `index.tsx` for the same reason as the text widget: the bare-Node
 * test runner cannot load JSX or `react-native`, and this is the half the
 * tests need.
 */

import {z} from "zod"

import {ResizeMode} from "../../../../layout/schema/style.ts"

import type {FieldDescriptor} from "../../../../layout/registry/fields.ts"
import type {WidgetDefinition} from "../../../../layout/registry/registry.ts"

/** Matches `Widget.type` on the wire. */
export const IMAGE_TYPE = "image"

/** What an image with no `resizeMode` uses: the whole picture, undistorted. */
export const DEFAULT_RESIZE_MODE = "contain"

export const ImageConfigSchema = z.object({
    // `z.string()`, deliberately NOT `.url()`. `data:` URIs and relative asset
    // paths are legitimate sources and `.url()` rejects both. A URI that does
    // not resolve is a RENDER concern, not a parse one — the widget shows a
    // placeholder rather than the layout failing to load.
    uri: z.string(),
    // Reused from the style vocabulary rather than redeclared, so a widget's
    // image and a background image resize by the same rules.
    resizeMode: ResizeMode.optional(),
    // Screen-reader text. Also what a sighted user is told the missing image
    // was, because it is rendered behind the picture.
    alt: z.string().optional(),
}).strict()

/** Derived with `z.infer`, never an `interface` — see `../text/definition.ts`. */
export type ImageConfig = z.infer<typeof ImageConfigSchema>

const RESIZE_OPTIONS = ResizeMode.options.map((value) => ({
    label: value[0].toUpperCase() + value.slice(1),
    value,
}))

const FIELDS: FieldDescriptor[] = [
    {key: "uri", label: "Image", kind: "uri", accept: "image", required: true},
    {key: "resizeMode", label: "Fit", kind: "select", options: RESIZE_OPTIONS},
    {key: "alt", label: "Alt text", kind: "text"},
]

/**
 * An empty `uri` is a valid default: a widget created from a config panel has
 * no source until someone picks one, and the renderer already has to survive a
 * URI that does not load.
 */
export const IMAGE_DEFAULTS: ImageConfig = {
    uri: "",
    resizeMode: DEFAULT_RESIZE_MODE,
    alt: "",
}

/** The renderer-free half of the definition. `index.tsx` adds `Component`. */
export const imageDefinition: WidgetDefinition<ImageConfig> = {
    type: IMAGE_TYPE,
    schema: ImageConfigSchema,
    fields: FIELDS,
    defaults: IMAGE_DEFAULTS,
}
