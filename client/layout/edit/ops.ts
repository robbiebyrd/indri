import {canPlace} from "../grid/collision.ts"

import type {GridRect, GridSize} from "../grid/coords.ts"
import type {PlacedWidget} from "../grid/collision.ts"
import type {Placement} from "../schema/placement.ts"
import type {Style} from "../schema/style.ts"
import type {Widget} from "../schema/widget.ts"

/**
 * Builders for the `layout` action.
 *
 * Nothing here applies anything optimistically. The server re-validates every
 * op and the published delta is the ONLY confirmation an edit happened — the
 * editing host learns about its own edit exactly the way every other player
 * does. Applying locally first would put the host on a different code path
 * from everyone else, which is how the two quietly diverge.
 *
 * Field names and op values mirror `internal/handlers/actions/layout/op.go`.
 * The Go decoder rejects unknown keys PER OP, so an extra field here is an
 * error rather than something ignored — keep the shapes exact.
 */

export const LAYOUT_ACTION = "layout"

export type LayoutScope = "board" | "scene" | "widget"

/** The socket surface these builders need. `MessageHandler` satisfies it. */
export interface LayoutSocket {
    send(message: object): void
}

/** Everything a builder needs to address an edit. */
export interface EditContext {
    gameCode: string
    sceneId: string
    grid: GridSize
    /**
     * Grid-placed siblings at the level being edited. Absolute widgets are
     * deliberately absent: they are exempt from collision as both subject and
     * obstacle, so a move onto one is legal and must still be sent.
     */
    siblings: PlacedWidget[]
}

function send(ws: LayoutSocket, gameCode: string, op: string, fields: Record<string, unknown>): void {
    // `op` and `code` are spread first so a caller cannot displace them, and
    // the Go decoder would reject a duplicate key anyway.
    ws.send({action: LAYOUT_ACTION, op, code: gameCode, ...fields})
}

export function addWidget(ws: LayoutSocket, ctx: EditContext, widgetId: string, widget: Widget): void {
    send(ws, ctx.gameCode, "addWidget", {sceneId: ctx.sceneId, widgetId, widget})
}

export function removeWidget(ws: LayoutSocket, ctx: EditContext, widgetId: string): void {
    send(ws, ctx.gameCode, "removeWidget", {sceneId: ctx.sceneId, widgetId})
}

/**
 * Move or resize. Both write the same field, which is why there is no separate
 * resize builder.
 *
 * Returns false when the rect collides or is out of bounds, and sends nothing.
 * That check is UX only — it avoids a round trip that the server would reject
 * anyway. It is NOT a security boundary; `validateLayout` in Go is.
 */
export function setPlacement(
    ws: LayoutSocket,
    ctx: EditContext,
    widgetId: string,
    next: GridRect,
): boolean {
    if (!canPlace(next, widgetId, ctx.siblings, ctx.grid)) return false

    const placement: Placement = {kind: "grid", ...next}
    send(ws, ctx.gameCode, "setPlacement", {sceneId: ctx.sceneId, widgetId, placement})

    return true
}

/** Absolute placement is percentage-based and exempt from collision, so it always sends. */
export function setAbsolutePlacement(
    ws: LayoutSocket,
    ctx: EditContext,
    widgetId: string,
    placement: Extract<Placement, {kind: "absolute"}>,
): void {
    send(ws, ctx.gameCode, "setPlacement", {sceneId: ctx.sceneId, widgetId, placement})
}

/** Merges into the widget's config server-side; removing a key is not expressible. */
export function setWidgetConfig(
    ws: LayoutSocket,
    ctx: EditContext,
    widgetId: string,
    config: Record<string, unknown>,
): void {
    send(ws, ctx.gameCode, "setWidgetConfig", {sceneId: ctx.sceneId, widgetId, config})
}

export function setStyle(
    ws: LayoutSocket,
    ctx: EditContext,
    scope: LayoutScope,
    style: Style,
    widgetId?: string,
): void {
    send(ws, ctx.gameCode, "setStyle", {scope, style, ...scopeAddress(ctx, scope, widgetId)})
}

/** An empty source clears the script: the server deletes the key rather than storing "". */
export function setScript(
    ws: LayoutSocket,
    ctx: EditContext,
    scope: LayoutScope,
    source: string,
    widgetId?: string,
): void {
    send(ws, ctx.gameCode, "setScript", {scope, source, ...scopeAddress(ctx, scope, widgetId)})
}

export function setGrid(ws: LayoutSocket, ctx: EditContext, grid: GridSize): void {
    send(ws, ctx.gameCode, "setGrid", {grid})
}

/**
 * The ids a scope carries. `board` must carry NEITHER — the Go decoder rejects
 * a board-scoped op that includes an address, so sending one unconditionally
 * would fail every board edit.
 */
function scopeAddress(
    ctx: EditContext,
    scope: LayoutScope,
    widgetId?: string,
): Record<string, string> {
    if (scope === "board") return {}
    if (scope === "scene") return {sceneId: ctx.sceneId}
    if (widgetId === undefined) {
        throw new Error("a widget-scoped layout op needs a widgetId")
    }

    return {sceneId: ctx.sceneId, widgetId}
}

/**
 * A short, collision-free widget id.
 *
 * Ids are author-facing and appear in delta paths, so they stay short and
 * readable rather than being UUIDs. Uniqueness is checked against the ids
 * already present instead of trusting randomness: `addWidget` on an existing
 * id is an error server-side, not a replace, so a collision would surface as a
 * rejected edit rather than silently clobbering someone's widget.
 */
export function newWidgetId(type: string, taken: Iterable<string>): string {
    const existing = new Set(taken)
    const base = type.replace(/[^a-z0-9]/gi, "").slice(0, 8).toLowerCase() || "w"

    for (let n = 1; ; n++) {
        const id = `${base}${n}`
        if (!existing.has(id)) return id
    }
}
