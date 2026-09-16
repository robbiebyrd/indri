import type {GridRect, GridSize, PlacedWidget} from "./coords.ts"

/**
 * AABB collision test identical to react-grid-layout's collides().
 * Edge-touching is NOT a collision — strict `<` on all four comparisons.
 */
export function collides(a: GridRect, b: GridRect): boolean {
    return a.col < b.col + b.w && b.col < a.col + a.w
        && a.row < b.row + b.h && b.row < a.row + a.h
}

/**
 * Returns true if rect `r` (belonging to widget `id`) can be placed without
 * overlapping any grid-kind sibling.
 *
 * Rules:
 * - The widget is skipped against itself (same id).
 * - Absolute widgets are excluded from both sides: an absolute subject may go
 *   anywhere, and absolute obstacles are ignored.
 * - Only widgets with placement.kind === "grid" are considered obstacles.
 */
export function canPlace(r: GridRect, id: string, siblings: PlacedWidget[], _g: GridSize): boolean {
    // If the subject widget is absolute, it may go anywhere.
    const subject = siblings.find(s => s.id === id)
    if (subject !== undefined && subject.placement.kind === "absolute") {
        return true
    }

    for (const sibling of siblings) {
        // Never collide a widget with itself.
        if (sibling.id === id) continue
        // Absolute siblings are exempt from collision checks.
        if (sibling.placement.kind === "absolute") continue
        // Check AABB collision with this grid sibling.
        const sp = sibling.placement
        if (collides(r, {col: sp.col, row: sp.row, w: sp.w, h: sp.h})) {
            return false
        }
    }
    return true
}
