/**
 * Field descriptors: the hand-written description of what a widget's config
 * looks like to a human editing it.
 *
 * These are NOT derived from the zod schema. Zod v4 moved its internals from
 * `._def` to `._zod.def` and zod's own guidance to library authors is to treat
 * them as private, so a deriver would be reaching into an unstable surface and
 * would break on a patch release. Instead the descriptors are written by hand
 * and `assertDescriptorsMatchSchema` (see `./registry.ts`) is the mechanism
 * that keeps the two lists honest.
 *
 * Pure TypeScript on purpose: no react-native, no zod. A descriptor is data
 * about a field, not a rendering of one, so it has to load in bare Node for
 * the tests and be usable by any panel implementation.
 */

/** One choice in a `select`/`multiselect`. `value` is what lands in the config. */
export type FieldOption = {label: string; value: string}

/**
 * The parts every field has, regardless of kind.
 *
 * `required` is editor-facing only — it tells the panel to mark the input, not
 * whether the schema will accept the config. The schema remains the single
 * authority on validity, because it is the thing that runs against wire data.
 */
export type FieldBase = {
    /** Config key this descriptor edits. Must exist in the widget's schema. */
    key: string
    label: string
    description?: string
    required?: boolean
}

/**
 * A closed set of editors. Kinds are deliberately coarse — one kind maps to one
 * input control — because every kind added here is a control every config panel
 * must be able to draw.
 */
export type FieldDescriptor = FieldBase & (
    | {kind: "text"; multiline?: boolean; maxLength?: number}
    | {kind: "number"; min?: number; max?: number; step?: number}
    | {kind: "color"}
    | {kind: "boolean"}
    | {kind: "date"}
    | {kind: "select"; options: FieldOption[]}
    | {kind: "multiselect"; options: FieldOption[]}
    | {kind: "uri"; accept: "image" | "video"}
)

/** Every discriminator in `FieldDescriptor`, for exhaustive switches and tests. */
export type FieldKind = FieldDescriptor["kind"]
