/**
 * Parse and format helpers for the field pickers, with no renderer attached.
 *
 * Same split as everywhere else in the layout engine: the half that can be
 * decided without a screen lives in a plain `.ts` the bare-Node runner can load,
 * and the `.tsx` beside it is left holding only state and markup. A colour that
 * normalises wrongly or a date that cannot survive a round trip is a bug worth
 * a test, and none of those tests should need a renderer to run.
 *
 * NOTHING HERE DECIDES WHETHER A VALUE IS ACCEPTABLE. `coerceFieldValue` and
 * then the widget's own zod schema do that, and they run on every edit. What
 * these helpers do is narrower: produce a WELL-FORMED value from what a human
 * typed, which is the job `layout/edit/panel.ts` explicitly delegates to the
 * pickers ("a colour picker cannot emit a malformed colour"). A second opinion
 * on validity would be a third source of truth; a well-formed value is not.
 *
 * Pure TypeScript on purpose: no react, no react-native, no zod, and no `Date`
 * — see `joinIsoDate` for why the calendar is computed by hand.
 */

import type {FieldDescriptor, FieldOption} from "../../../../layout/registry/fields.ts"

// --- text -------------------------------------------------------------------

/**
 * The text an input shows for a config value.
 *
 * Anything that is not a string or a number becomes an empty box rather than
 * `"[object Object]"`: `isDrawableField` has already rejected composite values,
 * so reaching this with one means the descriptor is wrong, and showing the
 * wreckage would invite a human to edit it.
 */
export function toInputText(value: unknown): string {
    if (typeof value === "string") return value
    if (typeof value === "number" && Number.isFinite(value)) return String(value)

    return ""
}

/** A `text` descriptor's display rules, defaulted for any other kind. */
export function textLimits(field: FieldDescriptor): {multiline: boolean; maxLength?: number} {
    if (field.kind !== "text") return {multiline: false}

    return {multiline: field.multiline === true, maxLength: field.maxLength}
}

// --- colour -----------------------------------------------------------------

/**
 * `#rgb`, `#rrggbb` or the same without the hash. Case-insensitive; the output
 * is always lower case so two spellings of one colour compare equal.
 */
const HEX_COLOR = /^#?([0-9a-f]{3}|[0-9a-f]{6})$/i

/**
 * Normalise a hex colour, or report that it is not one.
 *
 * `undefined` means "not hex", NOT "not a colour". The text widget's schema is
 * a plain `z.string()` precisely because React Native also accepts named
 * colours, `rgb(...)` and `hsl(...)`, and re-deriving that grammar here would
 * reject values the platform renders perfectly well. So the colour picker
 * normalises what it recognises and passes everything else through untouched —
 * which is also why the swatch can only preview the hex case.
 */
export function normalizeHexColor(raw: string): string | undefined {
    const match = HEX_COLOR.exec(raw.trim())
    if (match === null) return undefined

    const digits = match[1].toLowerCase()
    if (digits.length === 6) return `#${digits}`

    // #rgb is shorthand for each digit doubled, not for each digit padded.
    return `#${digits[0]}${digits[0]}${digits[1]}${digits[1]}${digits[2]}${digits[2]}`
}

// --- number -----------------------------------------------------------------

/** The editor-facing bounds of a `number` descriptor. */
export interface NumberRange {
    min?: number
    max?: number
    /** How far one stepper tap moves the value. Never zero. */
    step: number
}

/** A `number` descriptor's bounds, defaulted for any other kind. */
export function numberRange(field: FieldDescriptor): NumberRange {
    if (field.kind !== "number") return {step: 1}

    const step = field.step

    // A zero or negative step would make the stepper a no-op or run backwards,
    // which is worse than ignoring a descriptor that declared one.
    return {min: field.min, max: field.max, step: step !== undefined && step > 0 ? step : 1}
}

/**
 * The number a human typed, or `undefined` if they did not type one.
 *
 * An empty box is `undefined` rather than `0`: writing a zero nobody typed is
 * the one thing a number input must never do.
 */
export function parseNumberText(raw: string): number | undefined {
    const text = raw.trim()
    if (text === "") return undefined

    const n = Number(text)

    return Number.isFinite(n) ? n : undefined
}

/** Pull `n` inside the descriptor's bounds. */
export function clampNumber(range: NumberRange, n: number): number {
    if (range.min !== undefined && n < range.min) return range.min
    if (range.max !== undefined && n > range.max) return range.max

    return n
}

/**
 * One stepper tap.
 *
 * CLAMPED, unlike typed text. A tap on a button is not an assertion about a
 * value, so a tap that would leave the range stops at the edge rather than
 * producing the rejection `coerceFieldValue` gives typed input — a control that
 * reports an error for pressing its own button is broken.
 *
 * The result is rounded because binary floating point cannot represent a 0.1
 * step: `0.1 + 0.2` is `0.30000000000000004`, and that would be written into
 * the layout and broadcast to every player verbatim.
 */
export function stepNumber(range: NumberRange, value: number, direction: 1 | -1): number {
    return clampNumber(range, round(value + direction * range.step))
}

/** Enough places for any step a human would author, none of the binary dust. */
function round(n: number): number {
    return Number(n.toFixed(10))
}

// --- date -------------------------------------------------------------------

/** The three boxes of a segmented date entry, exactly as they are typed. */
export interface DateParts {
    year: string
    month: string
    day: string
}

/** An untouched date entry. */
export const BLANK_DATE: DateParts = {year: "", month: "", day: ""}

/** Whether nothing has been typed — which a picker reports as "no date". */
export function isBlankDate(parts: DateParts): boolean {
    return parts.year.trim() === "" && parts.month.trim() === "" && parts.day.trim() === ""
}

const ISO_DATE = /^(\d{4})-(\d{2})-(\d{2})$/
const DIGITS = /^\d+$/

/**
 * Join three typed segments into `YYYY-MM-DD`, or refuse.
 *
 * Refuses anything that is not a REAL date, not merely anything out of range:
 * `2026-02-30` has a plausible month and a plausible day and does not exist.
 * A picker that could emit it would be the "malformed value" case `panel.ts`
 * says pickers are responsible for preventing.
 *
 * NO `Date` IS CONSTRUCTED. `new Date("2026-02-30")` rolls over into March and
 * `Date.UTC(26, ...)` silently means 1926 — both turn a refusal into a wrong
 * answer. The calendar is four lines; that bug would not be.
 */
export function joinIsoDate(parts: DateParts): string | undefined {
    const year = parts.year.trim()
    const month = parts.month.trim()
    const day = parts.day.trim()

    if (!DIGITS.test(year) || !DIGITS.test(month) || !DIGITS.test(day)) return undefined

    const y = Number(year)
    const m = Number(month)
    const d = Number(day)

    // Year 0 has no ISO-8601 representation in the four-digit form, and five
    // digits is a different format entirely.
    if (y < 1 || y > 9999) return undefined
    if (m < 1 || m > 12) return undefined
    if (d < 1 || d > daysInMonth(y, m)) return undefined

    return `${pad(y, 4)}-${pad(m, 2)}-${pad(d, 2)}`
}

/**
 * Split `YYYY-MM-DD` back into its segments, or refuse.
 *
 * Refuses everything `joinIsoDate` would refuse, by asking it: a stored value
 * that is not a real date must not load into the boxes as though it were, or
 * the picker would show a date the picker itself could never have produced.
 */
export function splitIsoDate(iso: string): DateParts | undefined {
    const match = ISO_DATE.exec(iso.trim())
    if (match === null) return undefined

    const parts: DateParts = {year: match[1], month: match[2], day: match[3]}

    return joinIsoDate(parts) === undefined ? undefined : parts
}

const DAYS_IN_MONTH = [31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31]

function daysInMonth(year: number, month: number): number {
    if (month === 2 && isLeapYear(year)) return 29

    return DAYS_IN_MONTH[month - 1]
}

function isLeapYear(year: number): boolean {
    return (year % 4 === 0 && year % 100 !== 0) || year % 400 === 0
}

function pad(n: number, width: number): string {
    return String(n).padStart(width, "0")
}

// --- choices ----------------------------------------------------------------

/** A `select`/`multiselect` descriptor's choices, empty for any other kind. */
export function fieldOptions(field: FieldDescriptor): readonly FieldOption[] {
    if (field.kind !== "select" && field.kind !== "multiselect") return []

    return field.options
}

/**
 * The selection a multiselect should draw, from whatever the config holds.
 *
 * ORDER IS THE STORED ORDER, never the option list's. The array records the
 * order the author chose things in, and re-sorting it to match the palette
 * would reshuffle a selection on every re-render and then write the reshuffle
 * back as an edit nobody made.
 *
 * Duplicates collapse and unrecognised values are dropped, so what is drawn is
 * exactly what the next toggle will emit. Keeping an unrecognised value would
 * be worse than dropping it: `coerceFieldValue` rejects it, so every subsequent
 * toggle would fail with an error about a value the panel never showed.
 */
export function selectedValues(value: unknown, options: readonly FieldOption[]): string[] {
    if (!Array.isArray(value)) return []

    const items: readonly unknown[] = value
    const allowed = new Set(options.map((option) => option.value))
    const chosen: string[] = []

    for (const item of items) {
        if (typeof item !== "string") continue
        if (!allowed.has(item)) continue
        if (chosen.includes(item)) continue

        chosen.push(item)
    }

    return chosen
}

/**
 * Add or remove one value.
 *
 * A new value goes on the END and a removal closes the gap, so the positions of
 * everything the author did not touch are untouched. Anything else — inserting
 * in option order, sorting the result — would make one tap rewrite the whole
 * array and publish a delta full of moves nobody asked for.
 */
export function toggleValue(current: readonly string[], value: string): string[] {
    if (current.includes(value)) return current.filter((item) => item !== value)

    return [...current, value]
}

/** A `uri` descriptor's accepted media, or `undefined` for any other kind. */
export function uriAccept(field: FieldDescriptor): "image" | "video" | undefined {
    return field.kind === "uri" ? field.accept : undefined
}
