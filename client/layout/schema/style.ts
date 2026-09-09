import {z} from "zod"

/**
 * The style vocabulary is closed and RN-expressible, NOT a CSS subset. Every
 * key here either maps to a real React Native style prop or compiles to a
 * background layer (see `../style/compile.ts`). There is deliberately no
 * `background-size`/`repeat`, no multiple backgrounds, no grid and no `calc()`,
 * because RN cannot express them and a key we cannot honour is worse than a
 * key that does not exist.
 */

/** How a background image fills its layer. Same set RN's ImageResizeMode uses. */
export const ResizeMode = z.enum(["cover", "contain", "stretch", "repeat", "center"])

/** Gradient endpoint in unit-square coordinates, the form expo-linear-gradient takes. */
export const Point = z.object({x: z.number(), y: z.number()}).strict()

/**
 * A scalar applies to all four corners; the object form is per-corner. Corners
 * are named individually rather than as a CSS shorthand because RN has one
 * property per corner and nothing to compile a shorthand down to.
 */
export const Radius = z.union([
    z.number().nonnegative(),
    z.object({
        topLeft: z.number().nonnegative().optional(),
        topRight: z.number().nonnegative().optional(),
        bottomRight: z.number().nonnegative().optional(),
        bottomLeft: z.number().nonnegative().optional(),
    }).strict(),
])

// Non-negative throughout: RN treats a negative radius or padding as invalid
// and the result is platform-dependent, so reject it at the edge rather than
// shipping a box that renders differently on iOS and web.
export const Edges = z.object({
    top: z.number().nonnegative().optional(),
    right: z.number().nonnegative().optional(),
    bottom: z.number().nonnegative().optional(),
    left: z.number().nonnegative().optional(),
}).strict()

/**
 * `.strict()` everywhere, at every level: layouts arrive as untyped wire data,
 * so a misspelled key must surface as a validation issue. Silently dropping it
 * would show the author a box that ignores half of what they wrote.
 */
export const StyleSchema = z.object({
    backgroundColor: z.string().optional(),
    backgroundImage: z.object({
        uri: z.string(),
        resizeMode: ResizeMode.optional(),
    }).strict().optional(),
    backgroundGradient: z.object({
        colors: z.array(z.string()).min(2),
        locations: z.array(z.number()).optional(),
        start: Point.optional(),
        end: Point.optional(),
    }).strict().optional(),
    border: z.object({
        width: z.number().nonnegative(),
        color: z.string(),
        style: z.enum(["solid", "dotted", "dashed"]).optional(),
        radius: Radius.optional(),
    }).strict().optional(),
    padding: z.union([z.number().nonnegative(), Edges]).optional(),
    opacity: z.number().min(0).max(1).optional(),
    // The unit is required: `boxShadow` is the one cross-platform shadow from RN
    // 0.76 on, but 0.79 rejects unitless lengths, so "1 1 black" would silently
    // paint nothing on device.
    boxShadow: z.string().regex(/\d(px|em|rem)/).optional(),
    overflow: z.enum(["visible", "hidden"]).optional(),
}).strict()

export type ResizeMode = z.infer<typeof ResizeMode>
export type Point = z.infer<typeof Point>
export type Radius = z.infer<typeof Radius>
export type Edges = z.infer<typeof Edges>
export type Style = z.infer<typeof StyleSchema>
