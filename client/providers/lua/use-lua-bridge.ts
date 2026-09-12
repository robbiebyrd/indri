/**
 * React ownership of one session's Lua runtime.
 *
 * The bridge itself is framework-free (`layout/lua/bridge.ts`); all this adds
 * is a lifetime — created when the socket appears, disposed when the component
 * that owns the socket goes away — and a subscription to the override layer so
 * a script painting something re-renders the board.
 */

import {useCallback, useEffect, useState, useSyncExternalStore} from "react"

import {LuaBridge} from "@/layout/lua/bridge"
import {EMPTY_OVERRIDES} from "@/layout/lua/overrides"

import type {Overrides} from "@/layout/lua/overrides"
import type {MessageHandler} from "@/services/message-handler"

/**
 * The bridge for `ws`, or undefined on the first render.
 *
 * CREATED IN AN EFFECT, NOT DURING RENDER. Construction subscribes to the
 * socket, and a render-time subscription would leak one bridge per discarded
 * render — which under StrictMode is every first render. Nothing is missed by
 * waiting: the socket is not open yet on mount, so the first keyframe cannot
 * have arrived.
 */
export function useLuaBridge(ws: MessageHandler): LuaBridge | undefined {
    const [bridge, setBridge] = useState<LuaBridge | undefined>(undefined)

    useEffect(() => {
        const created = new LuaBridge({socket: ws, state: ws})
        setBridge(created)
        return () => {
            setBridge(undefined)
            created.dispose()
        }
    }, [ws])

    return bridge
}

/**
 * The current presentation overrides, re-reading whenever a script writes one.
 *
 * `OverrideLayer.snapshot()` returns a stable reference until something
 * actually changes, which is exactly the contract `useSyncExternalStore` wants:
 * no re-render for a write that changed nothing.
 */
export function useOverrides(bridge: LuaBridge | undefined): Overrides {
    const subscribe = useCallback(
        (onStoreChange: () => void) => {
            if (bridge === undefined) return () => undefined
            return bridge.host.overrides.subscribe(onStoreChange)
        },
        [bridge],
    )

    const snapshot = useCallback(
        () => bridge?.host.overrides.snapshot() ?? EMPTY_OVERRIDES,
        [bridge],
    )

    return useSyncExternalStore(subscribe, snapshot, snapshot)
}
