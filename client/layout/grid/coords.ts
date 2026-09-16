export const MIN_DIM = 8
export const MAX_DIM = 4096

export type GridSize = {
    cols: number
    rows: number
}

export type GridRect = {
    col: number
    row: number
    w: number
    h: number
}

export type PercentBox = {
    left: `${number}%`
    top: `${number}%`
    width: `${number}%`
    height: `${number}%`
}

// A placed widget — only the fields needed for collision
export type PlacedWidget = {
    id: string
    placement: {kind: "grid"; col: number; row: number; w: number; h: number} | {kind: "absolute"}
}

/**
 * Convert a GridRect to percentage-based CSS box values relative to the grid.
 * No cell array is ever allocated — the grid is a coordinate space only.
 */
export function toPercentBox(r: GridRect, g: GridSize): PercentBox {
    const left = (r.col / g.cols) * 100
    const top = (r.row / g.rows) * 100
    const width = (r.w / g.cols) * 100
    const height = (r.h / g.rows) * 100
    return {
        left: `${left}%`,
        top: `${top}%`,
        width: `${width}%`,
        height: `${height}%`,
    }
}

/**
 * Validate a GridSize. Throws if cols or rows are outside [MIN_DIM, MAX_DIM].
 */
export function validateGridSize(g: GridSize): void {
    if (g.cols < MIN_DIM || g.cols > MAX_DIM) {
        throw new Error(
            `invalid grid cols ${g.cols}: must be between ${MIN_DIM} and ${MAX_DIM}`
        )
    }
    if (g.rows < MIN_DIM || g.rows > MAX_DIM) {
        throw new Error(
            `invalid grid rows ${g.rows}: must be between ${MIN_DIM} and ${MAX_DIM}`
        )
    }
}

/**
 * Validate that a GridRect has positive integer dimensions.
 * Throws if w or h are zero, negative, or fractional.
 */
function validateGridRect(r: GridRect): void {
    if (!Number.isInteger(r.w) || r.w <= 0) {
        throw new Error(`invalid rect w ${r.w}: must be a positive integer`)
    }
    if (!Number.isInteger(r.h) || r.h <= 0) {
        throw new Error(`invalid rect h ${r.h}: must be a positive integer`)
    }
}

/**
 * Clamp a GridRect so it fits within the grid bounds.
 * Throws if w or h are zero, negative, or fractional.
 */
export function clampRect(r: GridRect, g: GridSize): GridRect {
    validateGridRect(r)
    const col = Math.max(0, Math.min(r.col, g.cols - r.w))
    const row = Math.max(0, Math.min(r.row, g.rows - r.h))
    const w = Math.min(r.w, g.cols)
    const h = Math.min(r.h, g.rows)
    return {col, row, w, h}
}
