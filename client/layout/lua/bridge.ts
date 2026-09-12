/**
 * The bridge between a websocket session and the Lua runtime.
 *
 * LUA IS ASYMMETRIC, AND THIS FILE IS WHERE THAT IS ENFORCED.
 *
 * - OUTBOUND: `indri.send(action, payload)` becomes one websocket message.
 *   That is the only way a script reaches the server.
 * - INBOUND: THERE IS NONE. No websocket message is ever routed into Lua, and
 *   `MessageHandler.parsers` gains nothing here. A script learns about the world
 *   by observing the REDUCED game state through `stateChanged`, so it can never
 *   interpret a raw message differently from the reducer and drift away from
 *   authoritative state. `./events.ts` has the long-form reasoning.
 *
 * The bridge takes the two halves as narrow structural interfaces rather than
 * importing `MessageHandler`. That keeps the websocket out of the layout engine
 * — the dependency runs one way, from the app into here — and it is what lets
 * this module be exercised without React or a socket.
 */

import {BOARD_SCOPE, scopeKey} from "./events.ts"
import {LuaHost} from "./host-api.ts"
import {LuaRuntime} from "./runtime.ts"
import {MAX_SUBGRID_DEPTH} from "../schema/widget.ts"

import type {Scope} from "./events.ts"
import type {Game} from "../../models/models.ts"

/**
 * Which kind of update produced a state. NOT cosmetic: a keyframe is a resync
 * and clears the override layer, a delta is incremental and must not.
 */
export type StateKind = "keyframe" | "delta"

/**
 * The outbound half of the session. `MessageHandler` satisfies this
 * structurally, including its drop-a-message-sent-before-the-socket-is-open
 * behaviour, which the bridge deliberately does not second-guess: buffering
 * here would replay a script's action at an arbitrary later moment, against
 * game state that has since moved on.
 */
export interface OutboundSocket {
    send(message: object): void
}

/**
 * The inbound half: already-reduced game state, announced once per applied
 * update. `MessageHandler.observe` satisfies this.
 */
export interface StateSource {
    /** Returns an unsubscribe function. */
    observe(listener: (game: Game, kind: StateKind) => void): () => void
}

export interface LuaBridgeOptions {
    socket: OutboundSocket
    state: StateSource
    /** Where `indri.log` goes. Defaults to `console.log`. */
    log?: (...args: unknown[]) => void
    /** Every script failure, from any path. Defaults to `console.warn`. */
    onError?: (err: string) => void
    instructionBudget?: number
}

/** One script found in a layout, with the scope it belongs to. */
export interface ScriptEntry {
    readonly scope: Scope
    readonly src: string
}

export class LuaBridge {
    readonly runtime: LuaRuntime
    readonly host: LuaHost

    private readonly unobserve: () => void
    /** scopeKey -> the script currently instantiated for it. */
    private scripts: ReadonlyMap<string, ScriptEntry> = new Map()
    private sceneId: string | undefined
    private disposed = false

    constructor(opts: LuaBridgeOptions) {
        const onError = opts.onError ?? ((err: string) => console.warn(`lua: ${err}`))

        this.runtime = new LuaRuntime({
            onError,
            ...(opts.instructionBudget === undefined
                ? {}
                : {instructionBudget: opts.instructionBudget}),
        })

        this.host = new LuaHost(this.runtime, {
            // `action` is spread LAST so a payload key called "action" cannot
            // rename the message: the action a script asked for is the action
            // the server must see.
            send: (action, payload) => opts.socket.send({...payload, action}),
            activeSceneId: () => this.sceneId,
            ...(opts.log === undefined ? {} : {log: opts.log}),
        })

        // The result is deliberately dropped: `LuaRuntime.guard` has already
        // pushed any failure through `onError`, and reporting it twice would
        // make one broken install look like two.
        this.host.install()

        this.unobserve = opts.state.observe((game, kind) => {
            this.apply(game, kind)
        })
    }

    /**
     * Publish one reduced game state to the scripts.
     *
     * Scripts are synced FIRST so a script the same update introduced still
     * hears about the state that introduced it; a script registered a moment
     * later would have missed its own first `stateChanged`.
     *
     * `sceneChanged` fires AFTER `stateChanged`, and only when
     * `stage.currentScene` actually moved. The order is what makes the event
     * usable: a handler told "the scene is now X" can read `indri.state()` and
     * find a game that already agrees. The first keyframe counts as a change
     * (undefined -> the scene), so a script does not have to special-case its
     * own arrival. A scene going AWAY is not announced — there is no scene to
     * name, and every script has already had `stateChanged` for it.
     *
     * TWO CONSEQUENCES FOLLOW, AND SCRIPT AUTHORS HAVE TO KNOW THEM. A chunk's
     * top-level code runs before this update lands, so `indri.state()` there
     * returns the PREVIOUS snapshot — empty on the very first keyframe — and
     * anything it paints is wiped by the keyframe's override clear immediately
     * afterwards. Do the work in a `stateChanged` handler, which runs after
     * both. Neither is worth reordering for: the alternative costs a newly
     * added script its first state entirely, which is the harder bug to spot.
     */
    private apply(game: Game, kind: StateKind): void {
        if (this.disposed) return

        const previous = this.sceneId
        this.sceneId = game.stage?.currentScene
        this.syncScripts(game)
        this.host.applyGameState(game, kind)

        if (this.sceneId !== undefined && this.sceneId !== previous) {
            this.host.broadcast("sceneChanged", this.sceneId)
        }
    }

    /**
     * Re-instantiate the scripts a delta changed, and forget the scopes it
     * removed.
     *
     * SCRIPTS ARE NOT RE-EVALUATED PER DELTA. Re-running every chunk on every
     * update would discard script-local state and spend the instruction budget
     * on priming code, which is what makes the budget meaningless. Only a chunk
     * whose SOURCE TEXT differs is re-run.
     */
    private syncScripts(game: Game): void {
        const next = collectScripts(layoutOf(game))

        // Before instantiating, so a scope whose script was deleted loses its
        // handlers even though nothing re-runs for it.
        this.host.retain([...next.values()].map((entry) => entry.scope))

        for (const [key, entry] of next) {
            if (this.scripts.get(key)?.src === entry.src) continue
            this.host.instantiate(entry.scope, entry.src)
        }

        this.scripts = next
    }

    /** Release the runtime. Idempotent; later state updates are ignored. */
    dispose(): void {
        if (this.disposed) return
        this.disposed = true

        this.unobserve()
        this.host.dispose()
        this.runtime.dispose()
    }
}

/**
 * Every script in a raw layout, keyed by scope.
 *
 * Walks the UNTRUSTED wire data directly rather than going through
 * `parseLayout`: the only thing needed here is the script strings, and running
 * the full schema parse on every delta — for a result the renderer computes
 * again anyway — would pay for validation this module does not use. Everything
 * is shape-checked as it goes, and the recursion is depth-capped, so malformed
 * data yields fewer scripts rather than an exception.
 *
 * Nested sub-grid widgets are flattened into `widget:<sceneId>:<widgetId>`,
 * matching how `OverrideLayer` keys widget patches: widget ids are unique
 * within a scene at every depth.
 */
export function collectScripts(rawLayout: unknown): ReadonlyMap<string, ScriptEntry> {
    const out = new Map<string, ScriptEntry>()
    if (!isRecord(rawLayout)) return out

    addScript(out, BOARD_SCOPE, rawLayout.script)

    const scenes = rawLayout.scenes
    if (!isRecord(scenes)) return out

    for (const [sceneId, scene] of Object.entries(scenes)) {
        if (!isRecord(scene)) continue
        addScript(out, {kind: "scene", sceneId}, scene.script)
        collectWidgetScripts(out, sceneId, scene.widgets, 1)
    }

    return out
}

function collectWidgetScripts(
    out: Map<string, ScriptEntry>,
    sceneId: string,
    widgets: unknown,
    depth: number,
): void {
    if (depth > MAX_SUBGRID_DEPTH || !isRecord(widgets)) return

    for (const [widgetId, widget] of Object.entries(widgets)) {
        if (!isRecord(widget)) continue
        addScript(out, {kind: "widget", sceneId, widgetId}, widget.script)
        if (isRecord(widget.config)) {
            collectWidgetScripts(out, sceneId, widget.config.widgets, depth + 1)
        }
    }
}

/** An empty script is dropped: instantiating it would only cost a chunk load. */
function addScript(out: Map<string, ScriptEntry>, scope: Scope, src: unknown): void {
    if (typeof src !== "string" || src === "") return
    out.set(scopeKey(scope), {scope, src})
}

/** The canonical home of a game's layout — the same place `BoardView` reads. */
function layoutOf(game: Game): unknown {
    return game.data?.layout
}

function isRecord(value: unknown): value is Record<string, unknown> {
    return typeof value === "object" && value !== null && !Array.isArray(value)
}
