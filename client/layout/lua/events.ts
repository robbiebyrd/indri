/**
 * Script event registration and dispatch.
 *
 * This module is deliberately Lua-free: it stores opaque handler handles and
 * calls back into an `invoke` supplied by the host. That keeps the two rules
 * that actually matter — bubbling order and the re-entrancy guard — testable
 * without a VM, and keeps the VM plumbing in one place (`host-api.ts`).
 *
 * BUBBLING runs widget -> scene -> board, the same "most specific first"
 * order the DOM uses, and stops the moment a handler returns `false`. Only a
 * literal `false` stops it: a handler that falls off its end returns nil, and
 * treating that as "stop" would make every handler accidentally exclusive.
 *
 * RE-ENTRANCY IS REFUSED, NOT QUEUED. A host callback invoked from inside a Lua
 * handler (a press handler that presses something, a state observer that pokes
 * state) must not start a second dispatch: the second dispatch would run
 * against half-applied work from the first, and a handler pair that each
 * triggers the other would recurse until the JS stack gives out. The refusal is
 * reported in the result rather than thrown, so the caller can log it.
 */

/**
 * The complete event vocabulary.
 *
 * There is NO `"message"` event, and adding one would be a design error. Lua is
 * asymmetric: scripts send actions outbound, but inbound information reaches
 * them only by observing the reduced game state through `stateChanged`. A
 * script that read raw websocket messages could interpret one differently from
 * the reducer and desynchronise from authoritative state.
 */
export type LuaEvent = "stateChanged" | "sceneChanged" | "widgetPress"

export const LUA_EVENTS: readonly LuaEvent[] = ["stateChanged", "sceneChanged", "widgetPress"]

export function isLuaEvent(value: string): value is LuaEvent {
    return (LUA_EVENTS as readonly string[]).includes(value)
}

/**
 * Which chunk a handler belongs to, and which object an event is about. One
 * type for both, because bubbling is exactly the walk from an event's scope up
 * through the scopes that enclose it.
 */
export type Scope =
    | {readonly kind: "board"}
    | {readonly kind: "scene"; readonly sceneId: string}
    | {readonly kind: "widget"; readonly sceneId: string; readonly widgetId: string}

export const BOARD_SCOPE: Scope = Object.freeze({kind: "board"})

/** Stable identity for a scope, used as the registry key. */
export function scopeKey(scope: Scope): string {
    switch (scope.kind) {
        case "board":
            return "board"
        case "scene":
            return `scene:${scope.sceneId}`
        case "widget":
            return `widget:${scope.sceneId}:${scope.widgetId}`
    }
}

/** The scope itself, then every scope enclosing it, outermost last. */
export function bubblePath(scope: Scope): readonly Scope[] {
    switch (scope.kind) {
        case "board":
            return [BOARD_SCOPE]
        case "scene":
            return [scope, BOARD_SCOPE]
        case "widget":
            return [scope, {kind: "scene", sceneId: scope.sceneId}, BOARD_SCOPE]
    }
}

/**
 * Runs one handler. Returning `false` — and only `false` — stops the chain. A
 * handler that failed must report `true` so that one broken script cannot mute
 * the ones above it.
 */
export type Invoke<H> = (handler: H, scope: Scope) => boolean

export interface DispatchResult {
    /** How many handlers actually ran. */
    readonly invoked: number
    /** A handler returned `false` and the chain stopped early. */
    readonly stopped: boolean
    /** The call was refused because a dispatch was already in progress. */
    readonly reentrant: boolean
}

const REFUSED: DispatchResult = Object.freeze({invoked: 0, stopped: false, reentrant: true})

interface Registration<H> {
    readonly event: LuaEvent
    readonly handler: H
}

/**
 * The scope is stored beside its handlers rather than recovered from the key.
 * Scene and widget ids are author-supplied and may contain the `:` the key
 * joins on, so a key is a one-way hash for lookup, never a source of truth.
 */
interface ScopeEntry<H> {
    readonly scope: Scope
    readonly registrations: Registration<H>[]
}

export class EventBus<H> {
    private readonly byScope = new Map<string, ScopeEntry<H>>()
    private dispatching = false

    /** Handlers run in registration order within a scope. */
    register(scope: Scope, event: LuaEvent, handler: H): void {
        const key = scopeKey(scope)
        const existing = this.byScope.get(key)
        if (existing === undefined) this.byScope.set(key, {scope, registrations: [{event, handler}]})
        else existing.registrations.push({event, handler})
    }

    /** True when the scope has at least one handler. */
    has(scope: Scope): boolean {
        return this.byScope.has(scopeKey(scope))
    }

    /**
     * Forget one scope's handlers — the scope's chunk is being re-instantiated,
     * or the widget it belongs to is gone. The dropped handlers are returned so
     * the caller can release whatever they reference.
     */
    removeScope(scope: Scope): H[] {
        const key = scopeKey(scope)
        const removed = this.byScope.get(key)
        if (removed === undefined) return []
        this.byScope.delete(key)
        return removed.registrations.map((r) => r.handler)
    }

    /**
     * Keep only the named scopes and drop the rest. This is how a handler
     * registered for a widget that a delta removed stops being reachable: the
     * caller passes the scopes the current layout still contains.
     */
    retain(scopes: Iterable<Scope>): H[] {
        const keep = new Set<string>()
        for (const scope of scopes) keep.add(scopeKey(scope))

        const dropped: H[] = []
        for (const [key, entry] of this.byScope) {
            if (keep.has(key)) continue
            this.byScope.delete(key)
            for (const r of entry.registrations) dropped.push(r.handler)
        }
        return dropped
    }

    clear(): H[] {
        const dropped: H[] = []
        for (const entry of this.byScope.values()) {
            for (const r of entry.registrations) dropped.push(r.handler)
        }
        this.byScope.clear()
        return dropped
    }

    /** Deliver a targeted event, bubbling widget -> scene -> board. */
    dispatch(event: LuaEvent, target: Scope, invoke: Invoke<H>): DispatchResult {
        return this.run(bubblePath(target), event, invoke, true)
    }

    /**
     * Deliver to every registered scope.
     *
     * `stateChanged` and `sceneChanged` are announcements about the world, not
     * about one widget, so every script must hear them and no script may
     * suppress another's copy — a `false` return does not stop a broadcast.
     */
    broadcast(event: LuaEvent, invoke: Invoke<H>): DispatchResult {
        return this.run(this.registeredScopes(), event, invoke, false)
    }

    private registeredScopes(): readonly Scope[] {
        return [...this.byScope.values()].map((entry) => entry.scope)
    }

    private run(
        path: readonly Scope[],
        event: LuaEvent,
        invoke: Invoke<H>,
        stoppable: boolean,
    ): DispatchResult {
        if (this.dispatching) return REFUSED
        this.dispatching = true

        let invoked = 0
        let stopped = false
        try {
            for (const scope of path) {
                // A copy: a handler may register or remove handlers while it
                // runs, and mutating the live array mid-iteration would skip or
                // repeat its neighbours.
                const registrations = this.byScope.get(scopeKey(scope))?.registrations.slice()
                if (registrations === undefined) continue

                for (const registration of registrations) {
                    if (registration.event !== event) continue
                    invoked++
                    if (invoke(registration.handler, scope) === false && stoppable) {
                        stopped = true
                        break
                    }
                }
                if (stopped) break
            }
        } finally {
            this.dispatching = false
        }

        return {invoked, stopped, reentrant: false}
    }
}
