import type {GridRect, GridSize, PlacedWidget} from "../grid/coords.ts"
import {canPlace} from "../grid/collision.ts"

/**
 * Row-major first-fit: returns the first rect (w×h) that fits without
 * colliding with any sibling. Returns undefined if no free cell is found.
 *
 * The search is bounded to min(cols,64)×min(rows,64) to avoid O(n³) scans
 * across large grids (4096×4096 → 16.7M positions). On overflow, the caller
 * should fall back to absolute placement at 0,0.
 */
export function firstFree(
    size: {w: number; h: number},
    sibs: PlacedWidget[],
    g: GridSize,
): GridRect | undefined {
    const maxRow = Math.min(g.rows, 64)
    const maxCol = Math.min(g.cols, 64)
    for (let row = 0; row + size.h <= maxRow; row++) {
        for (let col = 0; col + size.w <= maxCol; col++) {
            const r: GridRect = {col, row, ...size}
            if (canPlace(r, "", sibs, g)) return r
        }
    }
    return undefined
}
