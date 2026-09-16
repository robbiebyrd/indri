import {z} from "zod"

// Absolute placement uses percentage strings ONLY — pixel values are unrepresentable.
// This is a type-level guarantee: any non-`${number}%` string fails the schema.
const Percent = z.custom<`${number}%`>(
    v => typeof v === "string" && /^-?\d+(\.\d+)?%$/.test(v as string)
)

export const GridPlacement = z.object({
    kind: z.literal("grid"),
    col: z.number().int().nonnegative(),
    row: z.number().int().nonnegative(),
    w: z.number().int().positive(),
    h: z.number().int().positive(),
})

export const AbsolutePlacement = z.object({
    kind: z.literal("absolute"),
    left: Percent,
    top: Percent,
    width: Percent,
    height: Percent,
    z: z.number().int().optional(),
})

export const Placement = z.discriminatedUnion("kind", [GridPlacement, AbsolutePlacement])
export type Placement = z.infer<typeof Placement>
export type GridPlacement = z.infer<typeof GridPlacement>
export type AbsolutePlacement = z.infer<typeof AbsolutePlacement>
