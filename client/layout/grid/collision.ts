/**
 * Axis-aligned bounding-box collision over grid rects.
 *
 * Two deliberate omissions, both of which look like missing features:
 *
 * 1. ABSOLUTELY POSITIONED WIDGETS ARE NOT HANDLED HERE, and that is the whole
 *    design. They are exempt from collision as BOTH subject and obstacle, and
 *    the way that exemption is expressed is that the caller never puts them in
 *    `siblings`. Do not add a `kind` check to this module — an absolute widget
 *    has no grid rect to test in the first place, and a check here would be a
 *    second, divergent source of truth for the same rule.
 *
 * 2. THIS MODULE IS GRID-LEVEL AGNOSTIC. Collision is scoped per grid level: a
 *    sub-grid child collides only with its own siblings, inside its parent's
 *    coordinate space. `siblings` and `g` must therefore describe one single
 *    level, and picking that level is the caller's job.
 */

import {isValidRect, isWithinBounds} from "./coords.ts"

import type {GridRect, GridSize} from "./coords.ts"

export type PlacedWidget = {id: string; rect: GridRect}

/**
 * The same four-comparison test react-grid-layout uses. Strict `<` on every
 * edge, so edge-touching is NOT a collision: a rect ending at col 3 sits flush
 * against one starting at col 3.
 *
 * This is pure geometry, so a rect DOES overlap its own coordinates — RGL folds
 * an identity check in here, we keep it out. Self-exemption belongs in
 * `canPlace`, keyed on id, because that is what lets a drag move a widget
 * without colliding with the position it is leaving.
 */
export function collides(a: GridRect, b: GridRect): boolean {
    return a.col < b.col + b.w && b.col < a.col + a.w
        && a.row < b.row + b.h && b.row < a.row + a.h
}

/**
 * Reject-and-snap-back, which is react-grid-layout's `preventCollision` mode
 * rather than its default push-cascade. A cascade would move widgets the user
 * did not touch, and a game board is authored by hand — a rejected drag that
 * snaps back is predictable, a cascade is not.
 *
 * `id` is the mover's own id so that dragging a widget does not collide with
 * the position it is leaving. Pass `""` when placing a brand-new widget, since
 * no placed widget can match it.
 */
export function canPlace(r: GridRect, id: string, siblings: PlacedWidget[], g: GridSize): boolean {
    if (!isValidRect(r) || !isWithinBounds(r, g)) return false

    for (const sibling of siblings) {
        if (sibling.id === id) continue
        if (collides(r, sibling.rect)) return false
    }

    return true
}
