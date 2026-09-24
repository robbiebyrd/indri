/**
 * Pure (no-UI) helpers used by picker components.
 * Kept separate so they can be imported in Node test files without pulling in
 * React Native.
 */

/**
 * Parse and normalize a hex colour string.
 * Accepts #rgb (shorthand) and #rrggbb. Returns the canonical #rrggbb form,
 * or undefined if the string is not a valid hex colour.
 */
export function parseColor(s: string): string | undefined {
    const t = s.trim()
    const short = /^#([0-9a-fA-F]{3})$/.exec(t)
    if (short) {
        const [r, g, b] = short[1].split("")
        return `#${r}${r}${g}${g}${b}${b}`.toLowerCase()
    }
    const long = /^#([0-9a-fA-F]{6})$/.exec(t)
    if (long) {
        return `#${long[1]}`.toLowerCase()
    }
    return undefined
}

/**
 * Clamp a number to [min, max]. Limits that are undefined are unconstrained.
 */
export function clampNumber(v: number, min?: number, max?: number): number {
    let r = v
    if (min !== undefined) r = Math.max(min, r)
    if (max !== undefined) r = Math.min(max, r)
    return r
}

/**
 * Parse an ISO date string (YYYY-MM-DD). Returns the canonical form if valid,
 * or undefined if the string cannot be parsed as a calendar date.
 */
export function parseDate(s: string): string | undefined {
    const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(s.trim())
    if (!m) return undefined
    const input = `${m[1]}-${m[2]}-${m[3]}`
    const d = new Date(`${input}T00:00:00Z`)
    if (isNaN(d.getTime())) return undefined
    const out = d.toISOString().slice(0, 10)
    // Reject dates that JS normalized (e.g. 2024-02-30 → 2024-03-01).
    if (out !== input) return undefined
    return out
}

/**
 * Deduplicate a string array, preserving the order of first occurrence.
 */
export function dedupeValues(values: string[]): string[] {
    const seen = new Set<string>()
    const result: string[] = []
    for (const v of values) {
        if (!seen.has(v)) {
            seen.add(v)
            result.push(v)
        }
    }
    return result
}
