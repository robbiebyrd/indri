import {z} from "zod"
import {StyleSchema} from "./style.ts"
import {MIN_DIM, MAX_DIM} from "../grid/coords.ts"
import {collides} from "../grid/collision.ts"
import {WidgetSchema, MAX_SUBGRID_DEPTH} from "./widget.ts"
import type {PlacedWidget, GridRect} from "../grid/coords.ts"

export const GridSizeSchema = z.object({
    cols: z.number().int().min(MIN_DIM).max(MAX_DIM),
    rows: z.number().int().min(MIN_DIM).max(MAX_DIM),
})

export const SceneSchema = z.object({
    style: StyleSchema.optional(),
    script: z.string().optional(),
    widgets: z.record(z.string(), WidgetSchema),
})

export const GameLayoutSchema = z.object({
    grid: GridSizeSchema,
    style: StyleSchema.optional(),
    script: z.string().optional(),
    scenes: z.record(z.string(), SceneSchema),
})

export type GameLayout = z.infer<typeof GameLayoutSchema>
export type Scene = z.infer<typeof SceneSchema>

export type LayoutIssue = {
    path: string
    message: string
}

/**
 * Walk any plain-JS value and return all key names encountered at any depth.
 * Used to detect reserved keys like "privateData" before accepting a layout.
 */
function collectKeys(value: unknown, path: string, out: {key: string; path: string}[]): void {
    if (value === null || typeof value !== "object") return
    for (const [k, v] of Object.entries(value as Record<string, unknown>)) {
        out.push({key: k, path: path ? `${path}.${k}` : k})
        collectKeys(v, path ? `${path}.${k}` : k, out)
    }
}

/**
 * Check whether `raw` contains a key named "privateData" at any depth.
 *
 * SanitizeDelta (server-side) strips any path containing a "privateData" segment
 * at any depth, producing silent undebuggable data loss on the client. Accepting
 * a layout that uses "privateData" as a key would make bugs impossible to diagnose.
 */
function findPrivateDataKeys(raw: unknown): {key: string; path: string}[] {
    const found: {key: string; path: string}[] = []
    collectKeys(raw, "", found)
    return found.filter(e => e.key === "privateData")
}

/**
 * Walk the widget tree recursively and emit issues for subgrids nested deeper than
 * MAX_SUBGRID_DEPTH. The schema accepts but does not validate config at depth 0 (passthrough),
 * so this semantic check is the enforcement mechanism.
 */
function findExcessiveSubgridDepth(
    path: string,
    widgets: Record<string, unknown>,
    currentDepth: number,
    issues: LayoutIssue[],
): void {
    for (const [id, w] of Object.entries(widgets)) {
        const widget = w as {type?: string; config?: {widgets?: Record<string, unknown>}}
        if (widget?.type !== "subgrid") continue
        const widgetPath = `${path}.${id}`
        if (currentDepth > MAX_SUBGRID_DEPTH) {
            issues.push({
                path: widgetPath,
                message: `Subgrid "${id}" at ${widgetPath} exceeds the maximum nesting depth of ${MAX_SUBGRID_DEPTH}`,
            })
        }
        // Recurse into nested widgets regardless — we want to report all violations
        const nested = widget.config?.widgets
        if (nested && typeof nested === "object") {
            findExcessiveSubgridDepth(widgetPath, nested as Record<string, unknown>, currentDepth + 1, issues)
        }
    }
}

/**
 * Check all grid-kind widgets in a scene for pairwise AABB overlaps.
 * Absolute widgets are excluded from collision checks on both sides (by design).
 */
function findOverlaps(scenePath: string, widgets: Record<string, unknown>): LayoutIssue[] {
    const issues: LayoutIssue[] = []

    // Collect grid-placed widgets
    const placed: PlacedWidget[] = []
    for (const [id, w] of Object.entries(widgets)) {
        const widget = w as {placement?: {kind: string; col?: number; row?: number; w?: number; h?: number}}
        if (!widget?.placement) continue
        const p = widget.placement
        if (p.kind === "grid" && p.col !== undefined && p.row !== undefined && p.w !== undefined && p.h !== undefined) {
            placed.push({
                id,
                placement: {kind: "grid", col: p.col, row: p.row, w: p.w, h: p.h},
            })
        }
        // Absolute widgets are excluded — they may overlap freely
    }

    // Pairwise collision check
    for (let i = 0; i < placed.length; i++) {
        for (let j = i + 1; j < placed.length; j++) {
            const a = placed[i]
            const b = placed[j]
            if (a.placement.kind !== "grid" || b.placement.kind !== "grid") continue
            const ra: GridRect = {col: a.placement.col, row: a.placement.row, w: a.placement.w, h: a.placement.h}
            const rb: GridRect = {col: b.placement.col, row: b.placement.row, w: b.placement.w, h: b.placement.h}
            if (collides(ra, rb)) {
                issues.push({
                    path: `${scenePath}.widgets`,
                    message: `Widgets "${a.id}" and "${b.id}" overlap in scene "${scenePath}"`,
                })
            }
        }
    }
    return issues
}

/**
 * Check all grid-kind widgets in a scene for out-of-bounds placement.
 * col+w > cols or row+h > rows means the widget extends beyond the grid.
 */
function findOutOfBounds(
    scenePath: string,
    widgets: Record<string, unknown>,
    gridCols: number,
    gridRows: number,
): LayoutIssue[] {
    const issues: LayoutIssue[] = []
    for (const [id, w] of Object.entries(widgets)) {
        const widget = w as {placement?: {kind: string; col?: number; row?: number; w?: number; h?: number}}
        if (!widget?.placement) continue
        const p = widget.placement
        if (p.kind !== "grid") continue
        if (p.col === undefined || p.row === undefined || p.w === undefined || p.h === undefined) continue
        if (p.col + p.w > gridCols || p.row + p.h > gridRows) {
            issues.push({
                path: `${scenePath}.widgets.${id}`,
                message: `Widget "${id}" placement (col:${p.col}, row:${p.row}, w:${p.w}, h:${p.h}) extends beyond the ${gridCols}×${gridRows} grid`,
            })
        }
    }
    return issues
}

/**
 * Parse a raw unknown value as a GameLayout.
 *
 * Never throws — bad data degrades to an issue list.
 * A blank board is a worse failure than a wrong one.
 *
 * Returns:
 * - `{layout, issues}` on schema success (issues may still be non-empty for semantic problems)
 * - `{issues}` (no layout) on schema failure or if a reserved key is found
 */
export function parseLayout(raw: unknown): {layout?: GameLayout; issues: LayoutIssue[]} {
    // 1. Reject any input containing a "privateData" key at any depth.
    //    SanitizeDelta strips any path containing a "privateData" segment, producing silent
    //    data loss. Accepting it here would make server-side sanitization bugs undebuggable.
    const privateDataHits = findPrivateDataKeys(raw)
    if (privateDataHits.length > 0) {
        return {
            issues: privateDataHits.map(hit => ({
                path: hit.path,
                message: `"privateData" is a reserved key that the server strips during delta sanitization. Using it in a layout causes silent data loss. Found at: ${hit.path}`,
            })),
        }
    }

    // 2. Schema parse
    const parsed = GameLayoutSchema.safeParse(raw)
    if (!parsed.success) {
        return {
            issues: [{
                path: "root",
                message: parsed.error.message,
            }],
        }
    }

    const layout = parsed.data
    const issues: LayoutIssue[] = []

    // 3. Semantic checks (do NOT throw — issues are collected and returned alongside layout)

    for (const [sceneName, scene] of Object.entries(layout.scenes)) {
        const scenePath = `scenes.${sceneName}`
        const widgets = scene.widgets as Record<string, unknown>

        // 3a. Overlap detection for grid-kind widget pairs
        issues.push(...findOverlaps(scenePath, widgets))

        // 3b. Out-of-bounds detection
        issues.push(...findOutOfBounds(scenePath, widgets, layout.grid.cols, layout.grid.rows))

        // 3c. Subgrid depth cap — emit issues for nesting exceeding MAX_SUBGRID_DEPTH
        findExcessiveSubgridDepth(`${scenePath}.widgets`, widgets, 1, issues)
    }

    return {layout, issues}
}
