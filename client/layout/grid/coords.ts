/**
 * Grid coordinate math.
 *
 * The board is a PURE COORDINATE SPACE. A 4096x4096 grid is 16.7 million
 * cells, so nothing here ever materialises a cell array, a row array, or any
 * per-cell structure — the grid is only ever a divisor. Widgets are rects that
 * become percentage boxes and get absolutely positioned, which means the cost
 * of the module is O(widgets) and completely independent of grid size.
 *
 * Pure TypeScript on purpose: no react-native, no zod. It has to run in bare
 * Node for the tests and on both web and native at runtime.
 */

export const MIN_DIM = 8
export const MAX_DIM = 4096

export type GridSize = {cols: number; rows: number}

export type GridRect = {col: number; row: number; w: number; h: number}

export type PercentBox = {left: string; top: string; width: string; height: string}

export function isValidGridSize(g: GridSize): boolean {
    return isDimension(g.cols) && isDimension(g.rows)
}

function isDimension(n: number): boolean {
    return Number.isInteger(n) && n >= MIN_DIM && n <= MAX_DIM
}

/**
 * A rect must land on whole cells. Fractional coordinates would make the
 * percentage boxes disagree with what an editor snaps to, and a zero or
 * negative span is not a shape at all — a zero-area widget is invisible but
 * still eats input, so it is rejected rather than clamped away silently.
 */
export function isValidRect(r: GridRect): boolean {
    return Number.isInteger(r.col) && r.col >= 0
        && Number.isInteger(r.row) && r.row >= 0
        && Number.isInteger(r.w) && r.w > 0
        && Number.isInteger(r.h) && r.h > 0
}

export function isWithinBounds(r: GridRect, g: GridSize): boolean {
    return r.col >= 0 && r.row >= 0 && r.col + r.w <= g.cols && r.row + r.h <= g.rows
}

/**
 * Convert a rect to the percentage box the renderer positions with.
 *
 * Percentages rather than pixels because the same layout has to survive any
 * viewport on web and native without a measurement pass.
 */
export function toPercentBox(r: GridRect, g: GridSize): PercentBox {
    return {
        left: `${(r.col / g.cols) * 100}%`,
        top: `${(r.row / g.rows) * 100}%`,
        width: `${(r.w / g.cols) * 100}%`,
        height: `${(r.h / g.rows) * 100}%`,
    }
}

/**
 * Force a rect inside the grid, preserving its size where possible and
 * shrinking only when it cannot fit.
 *
 * The result always satisfies `isValidRect` and `isWithinBounds` for a valid
 * grid, so callers can clamp untrusted wire data once and then stop checking.
 * That is why fractional and non-finite input is normalised here instead of
 * rejected: the caller already decided to keep the widget.
 */
export function clampRect(r: GridRect, g: GridSize): GridRect {
    const w = clampSpan(r.w, g.cols)
    const h = clampSpan(r.h, g.rows)

    return {col: clampOrigin(r.col, g.cols - w), row: clampOrigin(r.row, g.rows - h), w, h}
}

/** Non-finite input would propagate through every comparison below, so it collapses to the minimum. */
function clampSpan(v: number, max: number): number {
    if (!Number.isFinite(v)) return 1

    return Math.min(Math.max(1, Math.floor(v)), max)
}

function clampOrigin(v: number, max: number): number {
    if (!Number.isFinite(v)) return 0

    return Math.min(Math.max(0, Math.floor(v)), max)
}
