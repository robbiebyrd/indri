import {z} from "zod"

import {MAX_DIM, MIN_DIM} from "../grid/coords.ts"

/**
 * A percentage string like "42%" or "12.5%".
 *
 * Absolute placement is expressed in percentages and nothing else. Modelling it
 * as a branded string rather than a number makes a pixel value *unrepresentable*
 * instead of merely invalid: there is no way to write `left: 40` and have it
 * parse, so "no pixel positioning, only relative" is enforced by the type rather
 * than by a rule someone has to remember.
 */
export const Percent = z.custom<`${number}%`>(
    (v) => typeof v === "string" && /^-?\d+(\.\d+)?%$/.test(v),
    {message: "must be a percentage string such as \"25%\" — pixel values are not allowed"},
)

export type Percent = z.infer<typeof Percent>

/** Grid dimensions, constrained to the renderable range. */
export const GridSizeSchema = z.object({
    cols: z.int().min(MIN_DIM).max(MAX_DIM),
    rows: z.int().min(MIN_DIM).max(MAX_DIM),
}).strict()

/**
 * Where a widget sits, and how it shares space.
 *
 * THREE INDEPENDENT DECISIONS, deliberately not welded together:
 *
 *   kind     — grid cells, or percentages of the viewport.
 *   overlap  — whether this widget may share space with another.
 *   z        — who draws on top when two do overlap.
 *
 * They used to be one: `grid` meant "collides, no z" and `absolute` meant
 * "exempt, has z". That made "let this one widget sit on top of another" and
 * "position this widget in percentages" the same switch, which they are not.
 *
 * `overlap: true` exempts a widget from collision as BOTH subject and obstacle:
 * it neither blocks others nor is blocked. That is what absolute placement has
 * always done, and it is the only reading under which turning the flag on
 * actually lets you resize a widget over its neighbour — under a mutual rule
 * the neighbour would still refuse. Absolute placement implies it, so an
 * absolute widget needs no flag.
 *
 * `z` is absent by default, in which case the renderer falls back to the widget
 * map's insertion order. Existing layouts therefore render identically.
 */
export const Placement = z.discriminatedUnion("kind", [
    z.object({
        kind: z.literal("grid"),
        // Deliberately just `number`, not int/positive. A fractional, zero or
        // out-of-bounds cell is a sloppy VALUE, and `parseLayout` clamps it and
        // warns so the rest of the board still renders. Contrast the absolute
        // branch below, where a bare number is the wrong FORMAT entirely and
        // there is nothing sensible to recover.
        col: z.number(),
        row: z.number(),
        w: z.number(),
        h: z.number(),
        overlap: z.boolean().optional(),
        z: z.int().optional(),
    }).strict(),
    z.object({
        kind: z.literal("absolute"),
        left: Percent,
        top: Percent,
        width: Percent,
        height: Percent,
        // Accepted but redundant: absolute placement is always exempt. Allowed
        // so that toggling a widget between kinds does not have to strip it.
        overlap: z.boolean().optional(),
        z: z.int().optional(),
    }).strict(),
])

/**
 * Whether this widget is exempt from collision, as both subject and obstacle.
 *
 * The single place that rule is expressed. Absolute placement is exempt by
 * definition; a grid widget is exempt only if it says so.
 */
export function allowsOverlap(placement: z.infer<typeof Placement>): boolean {
    return placement.kind === "absolute" || placement.overlap === true
}

export type Placement = z.infer<typeof Placement>
export type GridPlacement = Extract<Placement, {kind: "grid"}>
export type AbsolutePlacement = Extract<Placement, {kind: "absolute"}>

/** Numeric value of a percentage string, e.g. "25%" -> 25. */
export function percentValue(p: Percent): number {
    return Number.parseFloat(p)
}
