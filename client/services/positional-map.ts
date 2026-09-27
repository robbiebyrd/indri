export type PositionalMap = Record<string, number>

export function buildPositionalMap(obj: Record<string, unknown>, prefix = ""): PositionalMap {
    const result: PositionalMap = {}
    const keys = Object.keys(obj).sort()

    for (let i = 0; i < keys.length; i++) {
        const key = keys[i]
        const path = prefix ? `${prefix}.${key}` : key
        result[path] = i

        const child = obj[key]
        if (child !== null && typeof child === "object" && !Array.isArray(child)) {
            const nested = buildPositionalMap(child as Record<string, unknown>, path)
            Object.assign(result, nested)
        }
    }

    return result
}

// resolvePath converts a positional integer-array path back to a dotted string
// by walking schema to determine at each level whether the container is an
// object (sort keys, look up by index) or array (pass index through raw).
export function resolvePath(path: number[], schema: unknown): string {
    const parts: string[] = []
    let current: unknown = schema

    for (const seg of path) {
        if (Array.isArray(current)) {
            // Array container: raw numeric index
            parts.push(String(seg))
            current = (current as unknown[])[seg]
        } else if (current !== null && typeof current === "object") {
            // Object container: look up key by sorted index
            const keys = Object.keys(current as object).sort()
            const key = keys[seg]
            if (key === undefined) {
                parts.push(String(seg))
                current = undefined
            } else {
                parts.push(key)
                current = (current as Record<string, unknown>)[key]
            }
        } else {
            parts.push(String(seg))
            current = undefined
        }
    }

    return parts.join(".")
}
