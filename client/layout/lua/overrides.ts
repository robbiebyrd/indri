import type {Style} from "../schema/style.ts"

export type OverrideMap = {
    board?: {style?: Style}
    scene?: {style?: Style}
    widgets: Record<string, {style?: Style; config?: Record<string, unknown>}>
}

/** Return a fresh, empty override map. */
export function emptyOverrides(): OverrideMap {
    return {widgets: {}}
}

/**
 * Merge server layout with local overrides.
 *
 * The server value is the authoritative base. The local override style is
 * composited on top: override properties win over server properties, but
 * fields present only in the server value are preserved.
 *
 * This produces a plain object that the render layer can use directly —
 * the caller still holds the two originals separately.
 */
export function applyOverrides<T extends {style?: Style}>(
    server: T,
    override?: {style?: Style},
): T {
    if (!override?.style) return server
    return {
        ...server,
        style: {...server.style, ...override.style},
    }
}
