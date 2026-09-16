import {z} from "zod"

const ResizeMode = z.enum(["contain", "cover", "stretch", "repeat", "center"])
const Point = z.object({x: z.number(), y: z.number()})
const Radius = z.union([
    z.number().nonnegative(),
    z.object({
        topLeft: z.number().nonnegative().optional(),
        topRight: z.number().nonnegative().optional(),
        bottomLeft: z.number().nonnegative().optional(),
        bottomRight: z.number().nonnegative().optional(),
    }),
])
const Edges = z.object({
    top: z.number().optional(),
    right: z.number().optional(),
    bottom: z.number().optional(),
    left: z.number().optional(),
})

export const StyleSchema = z.object({
    backgroundColor: z.string().optional(),
    backgroundImage: z.object({
        uri: z.string().url(),
        resizeMode: ResizeMode.optional(),
    }).optional(),
    backgroundGradient: z.object({
        colors: z.array(z.string()).min(2),
        locations: z.array(z.number()).optional(),
        start: Point.optional(),
        end: Point.optional(),
    }).optional(),
    border: z.object({
        width: z.number().nonnegative(),
        color: z.string(),
        style: z.enum(["solid", "dotted", "dashed"]).optional(),
        radius: Radius.optional(),
    }).optional(),
    padding: z.union([z.number(), Edges]).optional(),
    opacity: z.number().min(0).max(1).optional(),
    boxShadow: z.string().regex(/\d+(px|em|rem)/).optional(),
    overflow: z.enum(["visible", "hidden"]).optional(),
}).strict()

export type Style = z.infer<typeof StyleSchema>
export type ResizeModeType = z.infer<typeof ResizeMode>
