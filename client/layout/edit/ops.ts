import type {GridRect, GridSize, PlacedWidget} from "../grid/coords.ts"
import {canPlace} from "../grid/collision.ts"

export type Sender = {
    send(message: object): void
}

export type EditContext = {
    gameCode: string
    sceneId: string
    siblings: PlacedWidget[]
    grid: GridSize
}

/**
 * Move or resize a grid widget. Returns false and sends nothing if the
 * candidate rect collides with a sibling — the local check is UX only;
 * the server re-validates and is the authority.
 */
export function moveWidget(
    ws: Sender,
    ctx: EditContext,
    id: string,
    next: GridRect,
): boolean {
    if (!canPlace(next, id, ctx.siblings, ctx.grid)) return false
    ws.send({
        action: "layout",
        op: "setPlacement",
        code: ctx.gameCode,
        sceneId: ctx.sceneId,
        widgetId: id,
        placement: {kind: "grid", ...next},
    })
    return true
}

/**
 * Add a widget to the scene. The caller must supply a widgetId that does
 * not already exist in the scene — a collision silently overwrites.
 */
export function addWidget(
    ws: Sender,
    ctx: EditContext,
    widgetId: string,
    widget: Record<string, unknown>,
): void {
    ws.send({
        action: "layout",
        op: "addWidget",
        code: ctx.gameCode,
        sceneId: ctx.sceneId,
        widgetId,
        widget,
    })
}

export function removeWidget(
    ws: Sender,
    ctx: EditContext,
    widgetId: string,
): void {
    ws.send({
        action: "layout",
        op: "removeWidget",
        code: ctx.gameCode,
        sceneId: ctx.sceneId,
        widgetId,
    })
}

export function setWidgetConfig(
    ws: Sender,
    ctx: EditContext,
    widgetId: string,
    config: Record<string, unknown>,
): void {
    ws.send({
        action: "layout",
        op: "setWidgetConfig",
        code: ctx.gameCode,
        sceneId: ctx.sceneId,
        widgetId,
        config,
    })
}

export type StyleScope =
    | {scope: "board"}
    | {scope: "scene"; sceneId: string}
    | {scope: "widget"; sceneId: string; widgetId: string}

export function setStyle(
    ws: Sender,
    gameCode: string,
    scopeArgs: StyleScope,
    style: Record<string, unknown>,
): void {
    ws.send({
        action: "layout",
        op: "setStyle",
        code: gameCode,
        ...scopeArgs,
        style,
    })
}

export function setGrid(
    ws: Sender,
    gameCode: string,
    grid: GridSize,
): void {
    ws.send({
        action: "layout",
        op: "setGrid",
        code: gameCode,
        grid,
    })
}

export type ScriptScope =
    | {scope: "board"}
    | {scope: "scene"; sceneId: string}
    | {scope: "widget"; sceneId: string; widgetId: string}

export function setScript(
    ws: Sender,
    gameCode: string,
    scopeArgs: ScriptScope,
    source: string,
): void {
    ws.send({
        action: "layout",
        op: "setScript",
        code: gameCode,
        ...scopeArgs,
        source,
    })
}
