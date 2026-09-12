/**
 * Where a newly created widget goes.
 *
 * Pure rect math over the registry's data, with no react-native anywhere, so
 * the whole "what does the palette emit" question is answerable in bare Node.
 * `palette.tsx` is only the pressables; every decision it makes lives here.
 */

import {canPlace} from "../grid/collision.ts"
import {addWidget, newWidgetId} from "./ops.ts"

import type {GridRect, GridSize} from "../grid/coords.ts"
import type {PlacedWidget} from "../grid/collision.ts"
import type {WidgetDefinition} from "../registry/registry.ts"
import type {AbsolutePlacement, Percent, Placement} from "../schema/placement.ts"
import type {Widget} from "../schema/widget.ts"
import type {EditContext, LayoutSocket} from "./ops.ts"

/** Just the extent of a rect: what a caller knows before a position is chosen. */
export type GridSpan = {w: number; h: number}

/**
 * How far from the origin a first-fit search may look, along each axis.
 *
 * THE BOUND IS THE POINT. `MAX_DIM` is 4096, so an unbounded row-major scan of
 * a maximal grid is 16.7 MILLION candidate positions, each of them O(siblings)
 * — per placement, on a UI thread. This caps candidate ORIGINS at 64x64 = 4096
 * regardless of grid size, which is a fixed, small ceiling.
 *
 * Only the origin is bounded, not the rect: a widget placed at an origin inside
 * the window may extend past it, so a span wider than 64 is still placeable.
 * What the window gives up is free space that begins beyond row or column 64,
 * which `firstFree` reports as "nowhere" — see `placementFor` for what happens
 * next. Finding that far-flung slot is not worth 16.7M comparisons in a PoC;
 * not searching catastrophically is worth quite a lot.
 */
export const MAX_SEARCH_SPAN = 64

/**
 * The size a widget created from the palette starts at.
 *
 * A registry-wide constant rather than a per-widget one: `WidgetDefinition`
 * describes a widget's CONFIG, and a preferred size is a layout opinion that
 * would have to be honoured by every grid size from 8x8 up. Two cells square
 * is visible on the smallest legal grid and is trivial to drag afterwards.
 */
export const NEW_WIDGET_SIZE: GridSpan = {w: 2, h: 2}

/**
 * The first free rect for `size`, scanning row-major from the origin.
 *
 * Undefined means "no free slot within the search window" (see
 * `MAX_SEARCH_SPAN`), NOT "the grid is full" — the caller must have a fallback
 * rather than treating this as a failure.
 *
 * No cell array at any grid size, deliberately: the board is a coordinate
 * space, and materialising 4096x4096 cells to answer one placement question
 * would cost more than the scan it replaced.
 */
export function firstFree(size: GridSpan, sibs: PlacedWidget[], g: GridSize): GridRect | undefined {
    const lastRow = Math.min(g.rows - size.h, MAX_SEARCH_SPAN - 1)
    const lastCol = Math.min(g.cols - size.w, MAX_SEARCH_SPAN - 1)

    for (let row = 0; row <= lastRow; row++) {
        for (let col = 0; col <= lastCol; col++) {
            const rect: GridRect = {col, row, w: size.w, h: size.h}
            // `canPlace` is the single authority on legality, including bounds
            // and rect validity, so a nonsensical `size` fails here instead of
            // producing a rect nothing else would accept.
            if (canPlace(rect, "", sibs, g)) return rect
        }
    }

    return undefined
}

/**
 * Where to put a new widget: a grid slot if one is free, absolute at the origin
 * if not.
 *
 * The fallback is not a consolation prize — absolute placement is exempt from
 * collision as both subject and obstacle, so a widget that could not be fitted
 * into the grid still appears, on top of whatever is there, and can be dragged
 * or re-placed afterwards. Refusing to create the widget at all would be the
 * worse answer: the author asked for one and would get silence.
 */
export function placementFor(size: GridSpan, sibs: PlacedWidget[], g: GridSize): Placement {
    const rect = firstFree(size, sibs, g)
    if (rect !== undefined) return {kind: "grid", ...rect}

    return absoluteAtOrigin(size, g)
}

/**
 * The widget a palette selection creates, before it has an id.
 *
 * `config` is a copy of the definition's `defaults`, so nothing downstream can
 * write through to the registry's own object and change what the NEXT widget of
 * this type starts with. No `style` and no `script`: both are optional, and the
 * Go decoder rejects unknown keys per op, so sending an empty one would be a
 * rejected edit rather than a harmless extra.
 */
export function newWidget(def: WidgetDefinition, ctx: EditContext): Widget {
    return {
        type: def.type,
        placement: placementFor(NEW_WIDGET_SIZE, ctx.siblings, ctx.grid),
        config: {...def.defaults},
    }
}

/**
 * Build and send the `addWidget` op for a palette selection. Returns the id the
 * new widget was given so the caller can select it.
 *
 * `taken` must list EVERY widget id at this level, including absolutely placed
 * ones. `ctx.siblings` is not that list — absolute widgets are deliberately
 * absent from it — and generating an id against the grid-placed subset alone
 * would collide with an absolute widget, which the server rejects rather than
 * replaces.
 */
export function createWidget(
    ws: LayoutSocket,
    ctx: EditContext,
    def: WidgetDefinition,
    taken: Iterable<string>,
): string {
    const id = newWidgetId(def.type, taken)
    addWidget(ws, ctx, id, newWidget(def, ctx))

    return id
}

/** Percentages, because absolute placement has no pixel form (see `schema/placement.ts`). */
function absoluteAtOrigin(size: GridSpan, g: GridSize): AbsolutePlacement {
    return {
        kind: "absolute",
        left: "0%",
        top: "0%",
        width: percent(Math.min(size.w, g.cols) / g.cols),
        height: percent(Math.min(size.h, g.rows) / g.rows),
    }
}

function percent(fraction: number): Percent {
    return `${fraction * 100}%`
}
