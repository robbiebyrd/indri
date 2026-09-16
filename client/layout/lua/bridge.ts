/**
 * bridge.ts — wires a LuaSession to an outbound send function.
 *
 * The bridge is intentionally one-directional: Lua can SEND actions to the
 * server, but it never receives raw WebSocket messages. Inbound state changes
 * reach Lua only through the stateChanged event, after the reducer has already
 * applied them. This prevents a script from desynchronising from authoritative
 * state by interpreting a raw message differently from the reducer.
 */
import type {LuaSession} from "./host-api.ts"

const MAX_DEPTH = 8

/**
 * Convert an arbitrary JS value to a plain JSON-safe representation.
 *
 * Rules:
 *   - Depth > MAX_DEPTH → throws an error (not caught here — callers must handle)
 *   - Cyclic reference → throws an error (not caught here — callers must handle)
 *   - Objects/arrays are walked recursively
 *   - Primitive types (string, number, boolean, null/undefined) pass through
 *
 * Throws if the value is unsafe to serialise (cyclic or too deep).
 * Callers that need null-fallback should wrap in try/catch.
 */
export function toPlainJson(value: unknown, depth = 0): unknown {
    return _walk(value, depth, new Set<object>())
}

function _walk(value: unknown, depth: number, seen: Set<object>): unknown {
    if (depth > MAX_DEPTH) {
        throw new Error(`[bridge] toPlainJson: payload exceeded maximum nesting depth of ${MAX_DEPTH}`)
    }

    if (value === null || value === undefined) return null
    if (typeof value === "string" || typeof value === "number" || typeof value === "boolean") {
        return value
    }

    if (typeof value !== "object") {
        // Functions, symbols, etc. — not serialisable
        return null
    }

    if (seen.has(value)) {
        throw new Error("[bridge] toPlainJson: cyclic reference detected in payload")
    }
    seen.add(value)

    if (Array.isArray(value)) {
        const result = value.map((item) => _walk(item, depth + 1, seen))
        seen.delete(value)
        return result
    }

    const obj = value as Record<string, unknown>
    const result: Record<string, unknown> = {}
    for (const key of Object.keys(obj)) {
        result[key] = _walk(obj[key], depth + 1, seen)
    }
    seen.delete(value)
    return result
}

/**
 * Attach the bridge: wire session.installHostApi's sendFn to the provided
 * outbound send function, applying toPlainJson payload safety.
 *
 * The sendFn is expected to behave like MessageHandler.send — it drops
 * silently if the socket is not open. Any error thrown by sendFn is caught and
 * logged so it does not propagate back into a Lua callback.
 *
 * If toPlainJson rejects the payload (cyclic or too deep), the send is dropped
 * with a warning rather than crashing the Lua callback.
 */
export function attachBridge(
    session: LuaSession,
    sendFn: (msg: object) => void,
): void {
    session.installHostApi((action: string, payload: Record<string, unknown>) => {
        let safePayload: Record<string, unknown>
        try {
            safePayload = (toPlainJson(payload) as Record<string, unknown>) ?? {}
        } catch (e) {
            console.warn("[bridge] attachBridge: payload rejected, dropping send:", e)
            return
        }
        sendFn({action, ...safePayload})
    })
}
