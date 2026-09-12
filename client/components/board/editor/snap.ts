/**
 * Gesture pixels -> grid cells.
 *
 * Everything the editor needs to turn a drag into a candidate `GridRect` lives
 * here, and nothing else does. The gesture layer above is untestable without a
 * React renderer (there is none in this repo), so the arithmetic is pulled out
 * to where `node --experimental-strip-types` can reach it — the same split
 * `components/board/placement.ts` already makes.
 *
 * Pure TypeScript on purpose: no react-native, no reanimated, no gesture
 * handler. It has to load in bare Node for its test.
 *
 * NOTE FOR THE UI THREAD: none of these are worklets, and they must not become
 * worklets casually — `snapMove`/`snapResize` would then have to call
 * `clampRect` and friends from `layout/grid/coords.ts` on the UI thread, which
 * would mean annotating that module too. The editor instead crosses to JS once
 * per gesture (a pixel delta, on `onEnd`) and snaps here. That still satisfies
 * "one op per gesture, never per frame", which is the constraint that matters.
 */

import type {GridRect, GridSize} from "../../../layout/grid/coords.ts"

/** Pixel size of the board, as reported by `onLayout`. */
export interface BoardSize {
    width: number
    height: number
}

/** Pixel size of one grid cell. */
export interface CellSize {
    width: number
    height: number
}

/** A widget frame in board-relative pixels. */
export interface PixelBox {
    left: number
    top: number
    width: number
    height: number
}

/**
 * Cell count above which the grid-line overlay is suppressed.
 *
 * A 4096x4096 grid would need 4095 vertical plus 4095 horizontal lines — 8190
 * Views for a decoration. Past this ceiling the editor prints the dimensions
 * as text instead, which is the only honest thing a line overlay could have
 * told the author at that density anyway.
 */
export const MAX_GRID_LINE_CELLS = 4096

export function showGridLines(grid: GridSize): boolean {
    return grid.cols * grid.rows <= MAX_GRID_LINE_CELLS
}

/**
 * Interior line positions for one axis, `1 .. n - 1`.
 *
 * The outer edges are the board's own border, so they are not lines. Only ever
 * called behind `showGridLines`, which is what keeps this from materialising a
 * 4095-element array.
 */
export function lineIndexes(n: number): number[] {
    const out: number[] = []
    for (let i = 1; i < n; i++) out.push(i)

    return out
}

export function cellSize(board: BoardSize, grid: GridSize): CellSize {
    return {width: board.width / grid.cols, height: board.height / grid.rows}
}

export function rectToPixels(r: GridRect, cell: CellSize): PixelBox {
    return {
        left: r.col * cell.width,
        top: r.row * cell.height,
        width: r.w * cell.width,
        height: r.h * cell.height,
    }
}

/**
 * Where a widget lands after being dragged by `(dx, dy)` pixels.
 *
 * Size is preserved and the origin is CLAMPED to the grid rather than
 * rejected: dragging past an edge should park the widget against that edge,
 * which is what every editor does, and rejecting there would make the whole
 * border of the board feel broken. Collision is a separate question and is not
 * asked here — `setPlacement` runs `canPlace` on the result and refuses to
 * send when it fails.
 */
export function snapMove(
    base: GridRect,
    dx: number,
    dy: number,
    cell: CellSize,
    grid: GridSize,
): GridRect {
    return {
        col: clampOrigin(base.col + cellsSpanned(dx, cell.width), grid.cols - base.w),
        row: clampOrigin(base.row + cellsSpanned(dy, cell.height), grid.rows - base.h),
        w: base.w,
        h: base.h,
    }
}

/**
 * Where a widget's bottom-right corner lands after the resize handle is
 * dragged by `(dx, dy)` pixels.
 *
 * The origin never moves — this is a corner handle, not a whole-rect
 * transform. The span is clamped to at least one cell (a zero-area widget is
 * invisible but still eats input) and at most the distance to the far edge.
 */
export function snapResize(
    base: GridRect,
    dx: number,
    dy: number,
    cell: CellSize,
    grid: GridSize,
): GridRect {
    return {
        col: base.col,
        row: base.row,
        w: clampSpan(base.w + cellsSpanned(dx, cell.width), grid.cols - base.col),
        h: clampSpan(base.h + cellsSpanned(dy, cell.height), grid.rows - base.row),
    }
}

/**
 * A pixel distance as a whole number of cells, rounding at the half cell.
 *
 * An unmeasured board has zero width, and a non-finite delta can arrive from a
 * gesture that was interrupted. Both collapse to "no movement" so the caller
 * gets its own rect back unchanged instead of `NaN` reaching a placement.
 */
function cellsSpanned(px: number, size: number): number {
    if (!Number.isFinite(px) || !Number.isFinite(size) || size <= 0) return 0

    return Math.round(px / size)
}

/** Both bounds are floored at 0 so a widget wider than the grid still yields a valid origin. */
function clampOrigin(v: number, max: number): number {
    return Math.min(Math.max(0, v), Math.max(0, max))
}

function clampSpan(v: number, max: number): number {
    return Math.min(Math.max(1, v), Math.max(1, max))
}
