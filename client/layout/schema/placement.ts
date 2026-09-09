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
 * Where a widget sits.
 *
 * `grid` placement is integer cells in the parent's coordinate space and
 * participates in collision. `absolute` placement is percentage-based, may
 * overlap freely, and is exempt from collision as both subject and obstacle —
 * an exemption expressed by never handing absolute widgets to the collision
 * module in the first place.
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
    }).strict(),
    z.object({
        kind: z.literal("absolute"),
        left: Percent,
        top: Percent,
        width: Percent,
        height: Percent,
        z: z.int().optional(),
    }).strict(),
])

export type Placement = z.infer<typeof Placement>
export type GridPlacement = Extract<Placement, {kind: "grid"}>
export type AbsolutePlacement = Extract<Placement, {kind: "absolute"}>

/** Numeric value of a percentage string, e.g. "25%" -> 25. */
export function percentValue(p: Percent): number {
    return Number.parseFloat(p)
}
