/**
 * The logic behind the widget config panel, with no renderer attached.
 *
 * Everything a config panel does other than draw boxes lives here: turning
 * what a picker produced into a config value, deciding whether that value is
 * acceptable, and collapsing a burst of keystrokes into one layout op. The
 * panel component (`components/board/editor/config-panel.tsx`) is then a thin
 * dispatch over `FieldDescriptor["kind"]`.
 *
 * Pure TypeScript on purpose: no react, no react-native. The bare-Node test
 * runner cannot load either, and this is the half worth testing. Only zod
 * *types* are referenced; the schema is called through the widget definition
 * that was handed in, so this module never imports zod either.
 */

import type {FieldDescriptor, FieldKind} from "../registry/fields.ts"
import type {WidgetDefinition} from "../registry/registry.ts"

/**
 * What a picker hands back.
 *
 * Deliberately wider than the config values it becomes. React Native's
 * `TextInput` reports strings even for a numeric keyboard, so a number picker
 * produces `"24"` and not `24`; `coerceFieldValue` is where that gap closes,
 * in one place, rather than in eight separate picker components.
 */
export type FieldInput = string | number | boolean | readonly string[] | null

/** A coerced value, or the message a human should be shown instead. */
export type Coerced =
    | {ok: true; value: unknown}
    | {ok: false; error: string}

/**
 * The single-key patch an edit turns into, or the message to display.
 *
 * `patch` holds exactly one key. `setWidgetConfig` merges server-side, so
 * sending the whole config would re-send every untouched key as part of the
 * broadcast delta — and would clobber a concurrent edit by another host to a
 * key this panel merely read.
 */
export type ConfigEdit =
    | {ok: true; key: string; value: unknown; patch: Record<string, unknown>}
    | {ok: false; error: string}

/**
 * Turn what a picker produced into the value that belongs in the config.
 *
 * The checks here are the DESCRIPTOR's rules (its `min`, `max`, `maxLength`,
 * its option list), not the schema's. The schema still runs afterwards and
 * remains the authority on validity — see `buildConfigEdit`. The split exists
 * because a descriptor can be stricter than the schema in editor-facing ways
 * the schema has no business knowing about: `fontSize` is any positive number
 * to zod, but the descriptor caps it at 512 because a font larger than that is
 * a typo rather than an intention.
 */
export function coerceFieldValue(field: FieldDescriptor, raw: FieldInput): Coerced {
    switch (field.kind) {
        case "text": {
            if (typeof raw !== "string") return wrongType(field, "text")
            if (field.maxLength !== undefined && raw.length > field.maxLength) {
                return fail(field, `must be at most ${field.maxLength} characters`)
            }

            return {ok: true, value: raw}
        }

        case "number": {
            // An empty box is not zero. Coercing it to 0 would silently write a
            // value nobody typed, so it is reported like any other bad input.
            if (typeof raw === "string" && raw.trim() === "") return fail(field, "must be a number")

            const n = typeof raw === "number" ? raw : typeof raw === "string" ? Number(raw.trim()) : NaN
            if (!Number.isFinite(n)) return fail(field, "must be a number")
            if (field.min !== undefined && n < field.min) return fail(field, `must be at least ${field.min}`)
            if (field.max !== undefined && n > field.max) return fail(field, `must be at most ${field.max}`)

            return {ok: true, value: n}
        }

        // Colours, dates and URIs are all strings on the wire and all have
        // their own format rules, but those rules belong to the picker that
        // produces them (a colour picker cannot emit a malformed colour) and to
        // the widget's schema. Re-deriving them here would be a third opinion.
        case "color":
        case "date":
        case "uri": {
            if (typeof raw !== "string") return wrongType(field, "text")

            return {ok: true, value: raw}
        }

        case "boolean": {
            if (typeof raw !== "boolean") return wrongType(field, "a yes/no value")

            return {ok: true, value: raw}
        }

        case "select": {
            if (typeof raw !== "string") return wrongType(field, "text")
            if (!field.options.some((o) => o.value === raw)) {
                return fail(field, `does not accept "${raw}"`)
            }

            return {ok: true, value: raw}
        }

        case "multiselect": {
            if (!Array.isArray(raw)) return wrongType(field, "a list of choices")

            const items: readonly unknown[] = raw
            const chosen: string[] = []
            for (const item of items) {
                if (typeof item !== "string") return wrongType(field, "a list of choices")
                if (!field.options.some((o) => o.value === item)) {
                    return fail(field, `does not accept "${item}"`)
                }
                // Deduped, order preserved: the picker reports a selection set
                // and a repeated entry means the same choice, not two of it.
                if (!chosen.includes(item)) chosen.push(item)
            }

            return {ok: true, value: chosen}
        }
    }

    return unreachableKind(field)
}

/**
 * Coerce an edit and validate it against the widget's own schema.
 *
 * This is the one place a human types an arbitrary value into a layout, so it
 * is the one place the schema has to run on the client. Everything else in the
 * editor moves values the server already accepted.
 *
 * WHY THE CANDIDATE IS THE WHOLE MERGED CONFIG, BUT ONLY ONE KEY IS SENT: a
 * strict object schema cannot parse a single key in isolation — every required
 * key would be reported missing. So the coerced value is merged over `current`
 * and the result is parsed, while the patch that goes on the wire stays a
 * single key.
 *
 * WHY ONLY ISSUES ON THE EDITED KEY BLOCK THE SEND: `current` came from the
 * server and may already be failing elsewhere (an older layout, a key another
 * host is mid-edit on). Refusing this edit because of somebody else's broken
 * key would wedge the panel with no way out, and would report a problem the
 * person typing did not cause and cannot fix from this input.
 */
export function buildConfigEdit<C extends Record<string, unknown>>(
    def: Pick<WidgetDefinition<C>, "schema">,
    current: Record<string, unknown>,
    field: FieldDescriptor,
    raw: FieldInput,
): ConfigEdit {
    const coerced = coerceFieldValue(field, raw)
    if (!coerced.ok) return {ok: false, error: coerced.error}

    const parsed = def.schema.safeParse({...current, [field.key]: coerced.value})
    if (!parsed.success) {
        const mine = parsed.error.issues.filter((issue) => issue.path[0] === field.key)
        if (mine.length > 0) {
            return {ok: false, error: `${field.label} ${mine.map((i) => i.message).join("; ")}`}
        }
    }

    return {ok: true, key: field.key, value: coerced.value, patch: {[field.key]: coerced.value}}
}

/**
 * Whether a panel can honestly draw this field.
 *
 * THE SUB-GRID PROBLEM, AND WHY THIS IS A STRUCTURAL TEST RATHER THAN A NEW
 * FIELD KIND. The `subgrid` widget has two config keys with no honest
 * descriptor: `grid` is a `{cols, rows}` pair and `widgets` is a map of child
 * widgets. Story 018 declared both as `kind: "text"` with a description asking
 * a panel to special-case them, and flagged that as its weakest part.
 *
 * Adding composite kinds to `FieldDescriptor` was considered and rejected. The
 * union is documented as "one kind maps to one input control", and every kind
 * added is a control that EVERY config panel must then be able to draw. `grid`
 * would need a two-number kind used by exactly one widget, and `widgets` has
 * no control at all — its children are placed on the canvas, so the correct
 * panel behaviour is to not draw it. Two single-use kinds, one of which names
 * a control that does not exist, is a worse vocabulary than none.
 *
 * The test used instead is structural and carries no widget-specific
 * knowledge: a descriptor kind that edits a PRIMITIVE cannot edit a value that
 * is an object or an array. Both sub-grid keys fail it, as would any future
 * widget that makes the same mistake, and a descriptor that is merely wrong
 * (`kind: "text"` over a number) is caught by the same rule. The panel draws a
 * read-only note for such a field rather than skipping it, so the key is
 * visibly not-editable-here instead of silently absent.
 *
 * `multiselect` is the one kind whose value is legitimately composite.
 */
export function isDrawableField(field: FieldDescriptor, value: unknown): boolean {
    if (value === null || value === undefined) return true
    if (typeof value !== "object") return true

    return field.kind === "multiselect" && Array.isArray(value)
}

/**
 * The kinds whose edits are debounced.
 *
 * Free-text and numeric entry arrive one keystroke at a time; every other kind
 * produces a whole value per deliberate gesture and has nothing to coalesce, so
 * debouncing them would only add lag to a tap.
 */
export function isDebouncedKind(kind: FieldKind): boolean {
    return kind === "text" || kind === "number"
}

/**
 * How long a burst of keystrokes is allowed to accumulate.
 *
 * Every layout op fans out to every connected client, and each client
 * deep-clones and replays its whole game state per delta — so a per-keystroke
 * op is not one wasted message, it is one full state rebuild per player per
 * character typed.
 */
export const CONFIG_EDIT_DEBOUNCE_MS = 250

/**
 * The timer functions the coalescer uses, injectable so tests need no wall
 * clock. The handle is `unknown` because the host's `setTimeout` returns a
 * number on the web and a `Timeout` object under Node, and the coalescer never
 * looks inside it — it only hands it back to the same `Timers`.
 */
export interface Timers {
    set(run: () => void, ms: number): unknown
    clear(handle: unknown): void
}

const hostTimers: Timers = {
    set: (run, ms) => setTimeout(run, ms),
    // The one cast in this module, and it is contained: the handle came out of
    // `setTimeout` directly above.
    clear: (handle) => clearTimeout(handle as Parameters<typeof clearTimeout>[0]),
}

/**
 * Collapses a burst of config edits into a single `setWidgetConfig` op.
 *
 * Edits accumulate into one pending patch keyed by config key, so typing into
 * `text` and then nudging `fontSize` before the timer fires produces one op
 * carrying both — still only the keys that were actually touched.
 *
 * An immediate edit (a toggle, a dropdown) flushes the pending patch WITH it
 * rather than jumping the queue. Sending it separately would let a tap on a
 * checkbox overtake the half-typed text that preceded it, and the server
 * applies ops in arrival order.
 */
export class ConfigEditCoalescer {
    private pending: Record<string, unknown> = {}
    private timer: unknown = undefined

    // Written out rather than declared as constructor parameter properties:
    // Node's strip-only type removal cannot erase those, and this module has to
    // load under `node --experimental-strip-types` for the tests.
    private readonly emit: (patch: Record<string, unknown>) => void
    private readonly delayMs: number
    private readonly timers: Timers

    constructor(
        emit: (patch: Record<string, unknown>) => void,
        delayMs: number = CONFIG_EDIT_DEBOUNCE_MS,
        timers: Timers = hostTimers,
    ) {
        this.emit = emit
        this.delayMs = delayMs
        this.timers = timers
    }

    /** Queue an edit. `immediate` sends the accumulated patch straight away. */
    push(key: string, value: unknown, immediate = false): void {
        this.pending[key] = value
        this.cancelTimer()

        if (immediate) {
            this.flush()

            return
        }

        this.timer = this.timers.set(() => {
            this.timer = undefined
            this.flush()
        }, this.delayMs)
    }

    /**
     * Send whatever is pending now. Call it when the panel closes or the widget
     * selection changes, or the last thing typed is lost.
     */
    flush(): void {
        this.cancelTimer()

        const patch = this.pending
        if (Object.keys(patch).length === 0) return

        this.pending = {}
        this.emit(patch)
    }

    /** Drop whatever is pending without sending it. */
    cancel(): void {
        this.cancelTimer()
        this.pending = {}
    }

    /** Test/UI affordance: whether an edit is waiting on the timer. */
    hasPending(): boolean {
        return Object.keys(this.pending).length > 0
    }

    private cancelTimer(): void {
        if (this.timer === undefined) return
        this.timers.clear(this.timer)
        this.timer = undefined
    }
}

/**
 * The whole edit path in one call: coerce, validate, and queue for the wire.
 *
 * The returned `ConfigEdit` is how the caller learns what to display — a
 * rejected edit is reported to the person who typed it and never reaches the
 * coalescer, so nothing invalid can be on the wire even briefly.
 *
 * This exists so the chain from "a picker fired" to "an op is queued" is one
 * testable function rather than logic stranded inside a component.
 */
export function applyFieldEdit<C extends Record<string, unknown>>(
    def: Pick<WidgetDefinition<C>, "schema">,
    current: Record<string, unknown>,
    field: FieldDescriptor,
    raw: FieldInput,
    coalescer: ConfigEditCoalescer,
): ConfigEdit {
    const edit = buildConfigEdit(def, current, field, raw)
    if (edit.ok) coalescer.push(edit.key, edit.value, !isDebouncedKind(field.kind))

    return edit
}

function fail(field: FieldDescriptor, why: string): Coerced {
    return {ok: false, error: `${field.label} ${why}`}
}

function wrongType(field: FieldDescriptor, expected: string): Coerced {
    return fail(field, `expects ${expected}`)
}

/**
 * Reached only if a `FieldDescriptor` kind exists that the switch above does
 * not handle, which the `never` parameter makes a compile error first.
 */
function unreachableKind(field: never): Coerced {
    return {ok: false, error: `unsupported field kind: ${JSON.stringify(field)}`}
}
