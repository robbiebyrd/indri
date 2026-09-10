/**
 * The presentation override layer.
 *
 * Lua drives behaviour; the server owns state. A script therefore never writes
 * to the game — it writes here, to a local map of presentation patches that is
 * composited over the server layout at render time by `mergeOverrides`. The
 * delta stream and the scripts can never fight over the same bytes, because
 * they never touch the same object.
 *
 * TWO RULES MAKE THIS SAFE, AND BOTH ARE LOAD-BEARING:
 *
 * 1. OVERRIDES ARE CLEARED ON A KEYFRAME AND ONLY ON A KEYFRAME. A keyframe is
 *    a resync — whatever a script painted before it was computed from state
 *    that no longer applies, so it must not survive. A delta is incremental and
 *    must NOT wipe a script's work. `clearForKeyframe` is deliberately named so
 *    that a delta path calling it reads as obviously wrong.
 *
 * 2. `mergeOverrides` NEVER MUTATES THE SERVER LAYOUT. The parsed layout is
 *    shared with the renderer and re-merged on every change, so a mutation
 *    would be permanent and cumulative. The merge is structure-sharing: any
 *    subtree with no override applied comes back by reference, and a merge with
 *    no overrides at all returns the very layout it was given.
 */

import {SUBGRID_TYPE} from "../schema/widget.ts"

import type {GameLayout, SceneLayout} from "../schema/layout.ts"
import type {Style} from "../schema/style.ts"
import type {Widget} from "../schema/widget.ts"

/** A patch over one widget's presentation. Never its placement or its type. */
export interface WidgetOverride {
    readonly style?: Style
    readonly config?: Readonly<Record<string, unknown>>
}

/** A patch over the board's or a scene's presentation. */
export interface StyleOverride {
    readonly style?: Style
}

/**
 * Every local presentation patch, keyed by scope.
 *
 * Widgets are keyed scene-first because widget ids are unique *within a scene*,
 * not globally: two scenes may both own a widget called `title`, and a flat map
 * would let one script's patch leak onto the other's widget.
 */
export interface Overrides {
    readonly board: StyleOverride
    /** scene id -> patch */
    readonly scenes: Readonly<Record<string, StyleOverride>>
    /** scene id -> widget id -> patch */
    readonly widgets: Readonly<Record<string, Readonly<Record<string, WidgetOverride>>>>
}

export const EMPTY_OVERRIDES: Overrides = Object.freeze({
    board: Object.freeze({}),
    scenes: Object.freeze({}),
    widgets: Object.freeze({}),
})

/**
 * The mutable owner of the override map.
 *
 * `snapshot()` returns a value with stable identity: the same object until
 * something actually changes, and a brand new one afterwards. That is what lets
 * a renderer treat "did the reference change?" as "do I need to re-merge?".
 */
export class OverrideLayer {
    private current: Overrides = EMPTY_OVERRIDES
    private readonly listeners = new Set<() => void>()

    snapshot(): Overrides {
        return this.current
    }

    /** Returns an unsubscribe function. */
    subscribe(listener: () => void): () => void {
        this.listeners.add(listener)
        return () => {
            this.listeners.delete(listener)
        }
    }

    setBoardStyle(style: Style): void {
        this.commit({...this.current, board: {style: mergeStyle(this.current.board.style, style)}})
    }

    setSceneStyle(sceneId: string, style: Style): void {
        const existing = this.current.scenes[sceneId]
        this.commit({
            ...this.current,
            scenes: {...this.current.scenes, [sceneId]: {style: mergeStyle(existing?.style, style)}},
        })
    }

    setWidgetStyle(sceneId: string, widgetId: string, style: Style): void {
        this.patchWidget(sceneId, widgetId, (existing) => ({
            ...existing,
            style: mergeStyle(existing?.style, style),
        }))
    }

    setWidgetConfig(sceneId: string, widgetId: string, patch: Readonly<Record<string, unknown>>): void {
        this.patchWidget(sceneId, widgetId, (existing) => ({
            ...existing,
            config: mergeConfig(existing?.config, patch),
        }))
    }

    /**
     * Drop every patch. CALL THIS ON A KEYFRAME AND NOWHERE ELSE — see the rule
     * at the top of this file.
     */
    clearForKeyframe(): void {
        if (this.current === EMPTY_OVERRIDES) return
        this.commit(EMPTY_OVERRIDES)
    }

    private patchWidget(
        sceneId: string,
        widgetId: string,
        next: (existing: WidgetOverride | undefined) => WidgetOverride,
    ): void {
        const scene = this.current.widgets[sceneId]
        this.commit({
            ...this.current,
            widgets: {
                ...this.current.widgets,
                [sceneId]: {...scene, [widgetId]: next(scene?.[widgetId])},
            },
        })
    }

    private commit(next: Overrides): void {
        this.current = next
        for (const listener of this.listeners) listener()
    }
}

/**
 * Composite the override layer over the server layout, producing the layout the
 * renderer should draw.
 *
 * The patch wins for the keys it names and the server wins for everything else,
 * key by key — a script setting `opacity` must not silently drop the author's
 * `backgroundColor`.
 */
export function mergeOverrides(layout: GameLayout, overrides: Overrides): GameLayout {
    const style = mergeStyle(layout.style, overrides.board.style)

    let scenes = layout.scenes
    for (const [sceneId, scene] of Object.entries(layout.scenes)) {
        const merged = mergeScene(
            scene,
            overrides.scenes[sceneId]?.style,
            overrides.widgets[sceneId],
        )
        if (merged === scene) continue
        // Copy on first change only, so an untouched layout is returned as-is.
        if (scenes === layout.scenes) scenes = {...layout.scenes}
        scenes[sceneId] = merged
    }

    if (style === layout.style && scenes === layout.scenes) return layout
    return {...layout, ...(style === undefined ? {} : {style}), scenes}
}

function mergeScene(
    scene: SceneLayout,
    style: Style | undefined,
    patches: Readonly<Record<string, WidgetOverride>> | undefined,
): SceneLayout {
    const nextStyle = mergeStyle(scene.style, style)
    const nextWidgets = patches === undefined ? scene.widgets : mergeWidgets(scene.widgets, patches)

    if (nextStyle === scene.style && nextWidgets === scene.widgets) return scene
    return {...scene, ...(nextStyle === undefined ? {} : {style: nextStyle}), widgets: nextWidgets}
}

/**
 * Apply widget patches at every depth of one scene.
 *
 * Sub-grids are walked because that is where the interesting widgets live: a
 * tic-tac-toe cell is a child of a sub-grid, and styling those cells is the
 * whole point of a scene script.
 */
function mergeWidgets(
    widgets: Record<string, Widget>,
    patches: Readonly<Record<string, WidgetOverride>>,
): Record<string, Widget> {
    let out = widgets

    for (const [id, widget] of Object.entries(widgets)) {
        const merged = mergeWidget(widget, patches[id], patches)
        if (merged === widget) continue
        if (out === widgets) out = {...widgets}
        out[id] = merged
    }

    return out
}

function mergeWidget(
    widget: Widget,
    patch: WidgetOverride | undefined,
    patches: Readonly<Record<string, WidgetOverride>>,
): Widget {
    const style = mergeStyle(widget.style, patch?.style)
    let config = mergeConfig(widget.config, patch?.config)

    if (widget.type === SUBGRID_TYPE) {
        const children = subGridWidgets(config)
        if (children !== undefined) {
            const mergedChildren = mergeWidgets(children, patches)
            if (mergedChildren !== children) config = {...config, widgets: mergedChildren}
        }
    }

    if (style === widget.style && config === widget.config) return widget

    const next: Widget = {...widget}
    if (style !== undefined) next.style = style
    if (config !== undefined) next.config = config
    return next
}

function mergeStyle(base: Style | undefined, patch: Style | undefined): Style | undefined {
    if (patch === undefined) return base
    return base === undefined ? patch : {...base, ...patch}
}

function mergeConfig(
    base: Record<string, unknown> | undefined,
    patch: Readonly<Record<string, unknown>> | undefined,
): Record<string, unknown> | undefined {
    if (patch === undefined) return base
    return {...base, ...patch}
}

/**
 * A sub-grid's children, or undefined when the config is not shaped like one.
 * `parseLayout` normalises valid sub-grids, but this module also runs against
 * hand-built layouts in tests, so the shape is checked rather than assumed.
 */
function subGridWidgets(
    config: Record<string, unknown> | undefined,
): Record<string, Widget> | undefined {
    const widgets = config?.widgets
    if (typeof widgets !== "object" || widgets === null || Array.isArray(widgets)) return undefined
    return widgets as Record<string, Widget>
}
