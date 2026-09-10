/**
 * The widget registry: what gives `Widget.config` its meaning.
 *
 * `layout/schema/widget.ts` validates a widget's *structure* and leaves
 * `config` opaque, because each widget type owns the shape of its own config.
 * This module is where those shapes live, paired with the hand-written field
 * descriptors that drive a config panel.
 *
 * Pure data and zod types only: no react-native, and React appears as a TYPE
 * ONLY. `import type` is erased before execution, so this module still loads in
 * bare Node for the tests.
 */

import type {ComponentType} from "react"
import type {z} from "zod"

import type {Widget} from "../schema/widget.ts"
import type {FieldDescriptor} from "./fields.ts"

/**
 * What every widget renderer is handed.
 *
 * `config` is NOT pre-parsed into the widget's own config type. The registry
 * stores erased `WidgetDefinition`s, and a `ComponentType<P>` is contravariant
 * in `P`, so a component typed on a narrow config could not be stored in the
 * erased map at all. Each renderer therefore parses `widget.config` with its
 * own schema, which is the same contract the rest of this module already uses.
 */
export interface WidgetRenderProps {
    /** The widget's key in its parent's widget map. Stable across deltas. */
    id: string
    /** The whole widget, already structurally validated by `parseLayout`. */
    widget: Widget
}

/**
 * Everything the app needs to know about one widget type.
 *
 * `api()` (the Lua host functions) is deliberately ABSENT. It arrives with the
 * Lua host, and a stub now would be a placeholder for work that has not been
 * designed yet. It is additive: a later story adds the property without
 * changing any of the members below, so nothing written against this type
 * breaks.
 *
 * `C` is constrained to an index-signature-compatible object so that a
 * concrete `WidgetDefinition<TextConfig>` can be stored in the registry's
 * erased `WidgetDefinition` map. Derive `C` with `z.infer<typeof Schema>`;
 * declaring the config as an `interface` will NOT satisfy the constraint,
 * because TypeScript gives implicit index signatures to type aliases only.
 */
export interface WidgetDefinition<C extends Record<string, unknown> = Record<string, unknown>> {
    /** Matches `Widget.type` on the wire. Unique across the registry. */
    type: string
    /** Runtime validation of this widget's config. MUST be `.strict()`. */
    schema: z.ZodType<C>
    /** One descriptor per config key; drives an auto-drawn config panel. */
    fields: FieldDescriptor[]
    /** The config a freshly created widget of this type starts with. */
    defaults: C
    /**
     * The React Native component that draws this widget.
     *
     * OPTIONAL, and deliberately so. A definition may exist purely so that a
     * type is "known" to `parseLayout` and editable by a config panel before
     * anyone has drawn it, and keeping it optional is also what lets this
     * module stay importable from bare Node — nothing here ever *calls* React.
     * A registered type with no `Component` renders a visible placeholder
     * rather than nothing; see `components/board/widget-host.tsx`.
     */
    Component?: ComponentType<WidgetRenderProps>
}

// Insertion-ordered so `listWidgets` is stable, which keeps any UI built on it
// from reshuffling when an unrelated widget is registered.
const registry = new Map<string, WidgetDefinition>()

/**
 * Add a widget type.
 *
 * A duplicate `type` throws rather than replacing: two definitions claiming
 * one wire type means one of them silently never renders, and a startup crash
 * is far cheaper to diagnose than a widget that is mysteriously the wrong one.
 */
export function registerWidget<C extends Record<string, unknown>>(def: WidgetDefinition<C>): void {
    if (registry.has(def.type)) {
        throw new Error(`widget type "${def.type}" is already registered`)
    }
    registry.set(def.type, def)
}

/** Undefined for an unknown type — an unknown widget is a layout warning, not a crash. */
export function getWidget(type: string): WidgetDefinition | undefined {
    return registry.get(type)
}

export function listWidgets(): WidgetDefinition[] {
    return [...registry.values()]
}

/**
 * The set `parseLayout` takes as `knownWidgetTypes`.
 *
 * A fresh copy, so a caller holding the result cannot mutate the registry
 * through it. Callers that parse on every keyframe should hoist it.
 */
export function knownWidgetTypes(): ReadonlySet<string> {
    return new Set(registry.keys())
}

/** Test-only. The registry is module-global, so tests must reset it. */
export function clearRegistry(): void {
    registry.clear()
}

// A key no real config would ever declare. Used to prove the schema is strict:
// a non-strict schema silently drops unknown keys instead of reporting them,
// which would make the descriptor check below pass vacuously.
const STRICTNESS_PROBE_KEY = "__indri_strictness_probe__"

// Value used for probe keys. Its type is irrelevant — the probe only reads
// `unrecognized_keys` issues, and a type complaint about a key means the key
// is known, which is exactly what the probe is asking.
const PROBE_VALUE = "__indri_probe_value__"

/**
 * Assert that a widget's field descriptors and its config schema describe the
 * same set of keys, in both directions.
 *
 * This is the whole reason descriptors may be hand-written. Without it, adding
 * a key to the schema and forgetting the descriptor produces a config panel
 * that quietly cannot edit that key, and a descriptor for a key the schema
 * rejects produces a panel input whose value is thrown away on save.
 *
 * HOW THE SCHEMA'S KEYS ARE ENUMERATED, WITHOUT TOUCHING ZOD INTERNALS:
 *
 * 1. `descriptor -> schema` is exact and fully behavioural. Membership of a
 *    *named* key is decidable: parse `defaults` plus that key and see whether
 *    the strict schema reports it under `unrecognized_keys`. Every descriptor
 *    key is named, so this direction has no blind spot.
 * 2. `schema -> descriptor` needs enumeration rather than membership, and
 *    enumerating an unbounded name space is not decidable behaviourally. The
 *    key set is taken to be `Object.keys(defaults)`: `defaults` is the
 *    definition's declaration of the keys it accepts. That declaration is not
 *    free-floating — it is pinned to the schema from below, because a strict
 *    schema cannot parse `defaults` unless every REQUIRED key is present in
 *    it, and cannot parse it if a key is fictitious. So required keys are
 *    covered by construction, and only optional ones rest on the declaration.
 *
 * LIMIT: an optional key (or a key with a zod `.default()`, which is optional
 * on input) that is absent from `defaults` is invisible to this check. Closing
 * it would need either zod introspection or a third list to keep in sync,
 * whereas `defaults` is a list the definition already has. A new optional key
 * therefore has to be forgotten twice — in the descriptors and in the defaults
 * — before a config panel can silently lose it.
 *
 * Throws on the first disagreement; returns nothing on success.
 */
export function assertDescriptorsMatchSchema<C extends Record<string, unknown>>(
    def: WidgetDefinition<C>,
): void {
    const where = `widget "${def.type}"`

    // Everything below reads the schema through `defaults`, so invalid
    // defaults would produce misleading failures. Reject them first.
    const parsedDefaults = def.schema.safeParse(def.defaults)
    if (!parsedDefaults.success) {
        throw new Error(
            `${where}: defaults do not satisfy the widget's own schema: ` +
            formatIssues(parsedDefaults.error.issues),
        )
    }

    const descriptorKeys = def.fields.map((f) => f.key)
    const duplicate = descriptorKeys.find((k, i) => descriptorKeys.indexOf(k) !== i)
    if (duplicate !== undefined) {
        throw new Error(`${where}: two field descriptors both edit config key "${duplicate}"`)
    }

    const probe: Record<string, unknown> = {...def.defaults, [STRICTNESS_PROBE_KEY]: PROBE_VALUE}
    for (const key of descriptorKeys) {
        if (!(key in probe)) probe[key] = PROBE_VALUE
    }

    const probed = def.schema.safeParse(probe)
    const unrecognised = probed.success
        ? new Set<string>()
        : unrecognisedKeys(probed.error.issues)

    if (!unrecognised.has(STRICTNESS_PROBE_KEY)) {
        throw new Error(
            `${where}: config schema did not reject an unknown key, so it is not .strict(). ` +
            "A non-strict schema strips unknown keys instead of reporting them, which makes " +
            "this whole check pass vacuously; add .strict() to the schema.",
        )
    }

    for (const key of descriptorKeys) {
        if (unrecognised.has(key)) {
            throw new Error(
                `${where}: field descriptor "${key}" edits a config key the schema does not ` +
                "accept, so anything typed into it would be discarded on save",
            )
        }
    }

    const described = new Set(descriptorKeys)
    for (const key of Object.keys(def.defaults)) {
        if (!described.has(key)) {
            throw new Error(
                `${where}: config key "${key}" has no field descriptor, so no config panel ` +
                "can edit it",
            )
        }
    }
}

function unrecognisedKeys(issues: readonly z.core.$ZodIssue[]): Set<string> {
    const keys = new Set<string>()
    for (const issue of issues) {
        if (issue.code === "unrecognized_keys") {
            for (const key of issue.keys) keys.add(key)
        }
    }

    return keys
}

function formatIssues(issues: readonly z.core.$ZodIssue[]): string {
    return issues.map((i) => `${i.path.join(".") || "<root>"}: ${i.message}`).join("; ")
}
