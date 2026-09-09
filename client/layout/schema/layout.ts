import {z} from "zod"

import {clampRect, isValidRect, isWithinBounds} from "../grid/coords.ts"
import {collides} from "../grid/collision.ts"
import {GridSizeSchema} from "./placement.ts"
import {StyleSchema} from "./style.ts"
import {MAX_SUBGRID_DEPTH, SUBGRID_TYPE, SubGridConfigSchema, WidgetSchema} from "./widget.ts"

import type {GridRect, GridSize} from "../grid/coords.ts"
import type {SubGridConfig, Widget} from "./widget.ts"

/** Key name that must never appear in a layout. See RESERVED_KEY_REASON. */
const RESERVED_KEY = "privateData"

const RESERVED_KEY_REASON =
    "\"privateData\" is reserved: the server's SanitizeDelta strips any path containing " +
    "that segment at any depth, so the value would be silently deleted in transit and the " +
    "layout would differ from what was authored, with nothing to show why"

export interface SceneLayout {
    style?: z.infer<typeof StyleSchema>
    script?: string
    widgets: Record<string, Widget>
}

export interface GameLayout {
    grid: GridSize
    style?: z.infer<typeof StyleSchema>
    script?: string
    scenes: Record<string, SceneLayout>
}

export const SceneLayoutSchema = z.object({
    style: StyleSchema.optional(),
    script: z.string().optional(),
    // A MAP keyed by id, never an array — see widget.ts.
    widgets: z.record(z.string().min(1), WidgetSchema),
}).strict()

export const GameLayoutSchema = z.object({
    grid: GridSizeSchema,
    style: StyleSchema.optional(),
    script: z.string().optional(),
    scenes: z.record(z.string().min(1), SceneLayoutSchema),
}).strict()

/**
 * `error` means the layout could not be used at all and `layout` is undefined.
 * `warning` means the layout renders, with the named problem corrected or
 * tolerated. The split matters: a blank board is a worse failure than a board
 * with one widget in the wrong place.
 */
export type IssueSeverity = "error" | "warning"

export interface LayoutIssue {
    severity: IssueSeverity
    /** Dotted path into the layout, e.g. `scenes.board.widgets.title.placement`. */
    path: string
    message: string
}

export interface ParseResult {
    /** Undefined only when an `error`-severity issue was raised. */
    layout?: GameLayout
    issues: LayoutIssue[]
}

export interface ParseOptions {
    /**
     * Widget types the registry knows about. When omitted the type check is
     * skipped — this module must not depend on the registry, so the caller
     * supplies it.
     */
    knownWidgetTypes?: ReadonlySet<string>
    /**
     * Scene ids present in `stage.scenes`. Layout scenes and stage scenes are
     * edited independently, so a mismatch is a warning, not an error.
     */
    sceneIds?: ReadonlySet<string>
}

/**
 * Parse untrusted layout data.
 *
 * NEVER THROWS. This runs against wire data on every keyframe, and a thrown
 * exception here would take out the whole render. Everything degrades to an
 * issue instead.
 *
 * The returned layout is NORMALISED: every grid-placed rect is guaranteed
 * valid and within its level's bounds, because out-of-range rects are clamped
 * rather than rejected. Renderers can therefore trust the geometry without
 * re-checking it.
 */
export function parseLayout(raw: unknown, opts: ParseOptions = {}): ParseResult {
    const issues: LayoutIssue[] = []

    const reservedAt = findReservedKey(raw, "")
    if (reservedAt !== undefined) {
        issues.push({severity: "error", path: reservedAt, message: RESERVED_KEY_REASON})
        return {issues}
    }

    const parsed = GameLayoutSchema.safeParse(raw)
    if (!parsed.success) {
        for (const issue of parsed.error.issues) {
            issues.push({
                severity: "error",
                path: issue.path.join("."),
                message: issue.message,
            })
        }
        return {issues}
    }

    const layout = parsed.data as GameLayout

    for (const [sceneId, scene] of Object.entries(layout.scenes)) {
        if (opts.sceneIds && !opts.sceneIds.has(sceneId)) {
            issues.push({
                severity: "warning",
                path: `scenes.${sceneId}`,
                message: `layout scene "${sceneId}" has no matching entry in stage.scenes`,
            })
        }
        normaliseWidgets(scene.widgets, layout.grid, `scenes.${sceneId}.widgets`, 1, opts, issues)
    }

    return {layout, issues}
}

/**
 * Walk one grid level: clamp bad rects, flag unknown types and overlaps, then
 * recurse into sub-grids. Mutates `widgets` in place — it is already a private
 * copy produced by zod's parse, never the caller's object.
 */
function normaliseWidgets(
    widgets: Record<string, Widget>,
    grid: GridSize,
    path: string,
    depth: number,
    opts: ParseOptions,
    issues: LayoutIssue[],
): void {
    for (const [id, widget] of Object.entries(widgets)) {
        const widgetPath = `${path}.${id}`

        if (opts.knownWidgetTypes && !opts.knownWidgetTypes.has(widget.type)) {
            issues.push({
                severity: "warning",
                path: `${widgetPath}.type`,
                message: `unknown widget type "${widget.type}"; it will not render`,
            })
        }

        if (widget.placement.kind === "grid") {
            const rect: GridRect = {
                col: widget.placement.col,
                row: widget.placement.row,
                w: widget.placement.w,
                h: widget.placement.h,
            }
            if (!isValidRect(rect) || !isWithinBounds(rect, grid)) {
                const fixed = clampRect(rect, grid)
                issues.push({
                    severity: "warning",
                    path: `${widgetPath}.placement`,
                    message:
                        `rect ${describe(rect)} is invalid or outside the ` +
                        `${grid.cols}x${grid.rows} grid; clamped to ${describe(fixed)}`,
                })
                Object.assign(widget.placement, fixed)
            }
        }

        if (widget.type === SUBGRID_TYPE) {
            recurseSubGrid(widget, widgetPath, depth, opts, issues)
        }
    }

    reportOverlaps(widgets, path, issues)
}

function recurseSubGrid(
    widget: Widget,
    widgetPath: string,
    depth: number,
    opts: ParseOptions,
    issues: LayoutIssue[],
): void {
    if (depth >= MAX_SUBGRID_DEPTH) {
        issues.push({
            severity: "warning",
            path: `${widgetPath}.config`,
            message:
                `sub-grid nesting exceeds the maximum depth of ${MAX_SUBGRID_DEPTH}; ` +
                "children below this level are dropped to avoid unbounded recursion",
        })
        widget.config = {...widget.config, widgets: {}}
        return
    }

    const nested = SubGridConfigSchema.safeParse(widget.config)
    if (!nested.success) {
        issues.push({
            severity: "warning",
            path: `${widgetPath}.config`,
            message: `sub-grid config is malformed: ${nested.error.issues[0]?.message ?? "invalid"}`,
        })
        return
    }

    const config: SubGridConfig = nested.data
    normaliseWidgets(
        config.widgets, config.grid, `${widgetPath}.config.widgets`, depth + 1, opts, issues,
    )
    widget.config = config as unknown as Record<string, unknown>
}

/**
 * Flag overlapping grid-placed siblings.
 *
 * Absolute widgets are skipped entirely — they are exempt from collision as
 * both subject and obstacle, which is why they never enter the comparison set.
 * Overlaps are a warning, not an error: the board still renders, with z-order
 * falling back to insertion order.
 */
function reportOverlaps(
    widgets: Record<string, Widget>,
    path: string,
    issues: LayoutIssue[],
): void {
    const placed = Object.entries(widgets)
        .filter(([, w]) => w.placement.kind === "grid")
        .map(([id, w]) => ({id, rect: w.placement as GridRect}))

    for (let i = 0; i < placed.length; i++) {
        for (let j = i + 1; j < placed.length; j++) {
            if (collides(placed[i].rect, placed[j].rect)) {
                issues.push({
                    severity: "warning",
                    path: `${path}.${placed[i].id}`,
                    message:
                        `overlaps "${placed[j].id}"; non-absolute widgets are not meant to ` +
                        "overlap, and stacking order falls back to insertion order",
                })
            }
        }
    }
}

/** Depth-first scan for the reserved key, returning its dotted path if present. */
function findReservedKey(value: unknown, path: string): string | undefined {
    if (Array.isArray(value)) {
        for (let i = 0; i < value.length; i++) {
            const hit = findReservedKey(value[i], path ? `${path}.${i}` : String(i))
            if (hit !== undefined) return hit
        }
        return undefined
    }
    if (typeof value !== "object" || value === null) return undefined

    for (const [key, child] of Object.entries(value)) {
        const childPath = path ? `${path}.${key}` : key
        if (key === RESERVED_KEY) return childPath
        const hit = findReservedKey(child, childPath)
        if (hit !== undefined) return hit
    }
    return undefined
}

function describe(r: GridRect): string {
    return `(col ${r.col}, row ${r.row}, ${r.w}x${r.h})`
}
