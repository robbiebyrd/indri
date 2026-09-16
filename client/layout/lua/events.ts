export type LuaEvent = "stateChanged" | "sceneChanged" | "widgetPress"
// "message" is intentionally absent — Lua observes state, it does not receive messages.

export type LuaHandler = (...args: unknown[]) => unknown

type RegistryKey = `${string}:${LuaEvent}`

function key(scope: string, event: LuaEvent): RegistryKey {
    return `${scope}:${event}`
}

/**
 * Event registry with three-level bubbling: widget → scene → game.
 *
 * Handlers are keyed by (scope, event) pairs, where scope is one of:
 *   - a widget id
 *   - the literal string "scene"
 *   - the literal string "game"
 */
export class EventRegistry {
    private readonly handlers = new Map<RegistryKey, LuaHandler>()

    on(scope: "game" | "scene" | string, event: LuaEvent, fn: LuaHandler): void {
        this.handlers.set(key(scope, event), fn)
    }

    off(scope: "game" | "scene" | string, event: LuaEvent): void {
        this.handlers.delete(key(scope, event))
    }

    /**
     * Bubble the event: widgetId → "scene" → "game".
     *
     * Emission stops as soon as a handler returns exactly `false`.
     * A null widgetId skips the widget tier and starts at "scene".
     */
    emit(
        event: LuaEvent,
        _sceneId: string,
        widgetId: string | null,
        ...args: unknown[]
    ): void {
        const scopes: string[] = []
        if (widgetId !== null) scopes.push(widgetId)
        scopes.push("scene", "game")

        for (const scope of scopes) {
            const fn = this.handlers.get(key(scope, event))
            if (fn) {
                const result = fn(...args)
                if (result === false) break
            }
        }
    }

    /** Remove all handlers registered for the given widget id. */
    dropWidget(widgetId: string): void {
        for (const event of ["stateChanged", "sceneChanged", "widgetPress"] as LuaEvent[]) {
            this.handlers.delete(key(widgetId, event))
        }
    }
}
